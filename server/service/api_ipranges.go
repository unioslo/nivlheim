package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
)

type apiMethodIpRanges struct {
	db *sql.DB
}

func (vars *apiMethodIpRanges) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != httpGET {
		vars.ServeHTTPREST(w, req)
		return
	}

	fields, hErr := unpackFieldParam(req.FormValue("fields"),
		[]string{"ipRange", "ipRangeId", "comment", "useDns"})
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	rows, err := vars.db.Query("SELECT iprangeid, iprange, use_dns, comment " +
		"FROM ipranges ORDER BY iprange")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var ipRangeID int
		var ipRange, comment sql.NullString
		var useDNS sql.NullBool
		err = rows.Scan(&ipRangeID, &ipRange, &useDNS, &comment)
		if err != nil && err != sql.ErrNoRows {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		item := make(map[string]interface{})
		if fields["ipRangeId"] {
			item["ipRangeId"] = ipRangeID
		}
		if fields["ipRange"] {
			item["ipRange"] = jsonString(ipRange)
		}
		if fields["comment"] {
			item["comment"] = jsonString(comment)
		}
		if fields["useDns"] {
			item["useDns"] = useDNS.Bool
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type Wrapper struct {
		A []map[string]interface{} `json:"ipRanges"`
	}
	returnJSON(w, req, Wrapper{A: result})
}

func (vars *apiMethodIpRanges) ServeHTTPREST(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case httpPUT:
		match := regexp.MustCompile("/(\\d+)$").FindStringSubmatch(req.URL.Path)
		if match == nil {
			http.Error(w, "Missing ipRangeId in URL path", http.StatusUnprocessableEntity)
			return
		}
		ipRangeID, _ := strconv.Atoi(match[1])
		iprange, ok := verifyIpRangeParameter(w, req, vars.db, ipRangeID)
		if !ok {
			return
		}
		// Update
		res, err := vars.db.Exec("UPDATE ipranges SET iprange=$1, comment=$2, "+
			"use_dns=$3 WHERE iprangeid=$4", iprange, req.FormValue("comment"),
			isTrueish(req.FormValue("useDns")), ipRangeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rowsAffected == 0 {
			http.Error(w, "Not Found", http.StatusNotFound) // 404 Not Found
			return
		}
		http.Error(w, "", http.StatusNoContent) // 204 No Content

	case httpPOST:
		iprange, ok := verifyIpRangeParameter(w, req, vars.db, -1)
		if !ok {
			return
		}
		// Insert
		_, err := vars.db.Exec("INSERT INTO ipranges(iprange,comment,use_dns) "+
			"VALUES($1,$2,$3)", iprange, req.FormValue("comment"),
			req.FormValue("useDns") != "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "", http.StatusCreated) // 201 Created

	case httpDELETE:
		match := regexp.MustCompile("/(\\d+)$").FindStringSubmatch(req.URL.Path)
		if match == nil {
			http.Error(w, "Missing ipRangeId in URL path", http.StatusUnprocessableEntity)
			return
		}
		ipRangeID, _ := strconv.Atoi(match[1])
		res, err := vars.db.Exec("DELETE FROM ipranges WHERE iprangeid=$1", ipRangeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rowsAffected == 0 {
			http.Error(w, "Not Found", http.StatusNotFound) // 404 Not Found
			return
		}
		http.Error(w, "", http.StatusNoContent) // 204 OK

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func verifyIpRangeParameter(w http.ResponseWriter, req *http.Request,
	db *sql.DB, updatingId int) (string, bool) {
	iprange := req.FormValue("ipRange")
	if iprange == "" {
		http.Error(w, "Missing parameter: ipRange", http.StatusUnprocessableEntity)
		return "", false
	}
	ip, ipnet, err := net.ParseCIDR(iprange)
	if err != nil {
		http.Error(w, "{\"ipRange\":\"Wrong format, should be CIDR\"}",
			http.StatusUnprocessableEntity)
		return "", false
	}
	if !bytes.Equal(ip.To16(), ipnet.IP.To16()) {
		http.Error(w, fmt.Sprintf("{\"ipRange\":\"The ip address can't have bits set "+
			"to the right side of the netmask. Try %v\"}", ipnet.IP),
			http.StatusUnprocessableEntity)
		return "", false
	}
	// Verify that the new range is not contained within, or contains,
	// any of the existing ranges (except for the one being updated)
	var count int
	err = db.QueryRow("SELECT count(*) FROM ipranges WHERE "+
		"($1 <<= iprange OR $1 >> iprange) AND iprangeid != $2",
		iprange, updatingId).Scan(&count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return "", false
	}
	if count > 0 {
		http.Error(w, "{\"ipRange\":\"This range is contained within, "+
			"or contains, one of the other ranges.\"}",
			http.StatusUnprocessableEntity)
		return "", false
	}
	return iprange, true
}
