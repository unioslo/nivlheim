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
	if req.Method != "GET" {
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
		"FROM ipranges ORDER BY iprangeid")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var ipRangeID int
		var ipRange, comment sql.NullString
		var useDns sql.NullBool
		err = rows.Scan(&ipRangeID, &ipRange, &useDns, &comment)
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
			item["useDns"] = useDns.Bool
		}
		result = append(result, item)
	}

	type Wrapper struct {
		A []map[string]interface{} `json:"ipRanges"`
	}
	returnJSON(w, req, Wrapper{A: result})
}

func (vars *apiMethodIpRanges) ServeHTTPREST(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "POST":
		iprange := req.FormValue("ipRange")
		if iprange == "" {
			http.Error(w, "Missing parameter: ipRange", http.StatusUnprocessableEntity)
			return
		}
		ip, ipnet, err := net.ParseCIDR(iprange)
		if err != nil {
			http.Error(w, "{\"ipRange\":\"Wrong format, should be CIDR\"}",
				http.StatusUnprocessableEntity)
			return
		}
		if !bytes.Equal(ip.To16(), ipnet.IP.To16()) {
			http.Error(w, fmt.Sprintf("{\"ipRange\":\"The ip address can't have bits set "+
				"to the right side of the netmask. Try %v\"}", ipnet.IP),
				http.StatusUnprocessableEntity)
			return
		}
		// Verify that the new range is not contained within, or contains,
		// any of the existing ranges.
		var count int
		vars.db.QueryRow("SELECT count(*) FROM ipranges WHERE $1 <<= iprange "+
			"OR $1 >> iprange", iprange).Scan(&count)
		if count > 0 {
			http.Error(w, "{\"ipRange\":\"This range is contained within, "+
				"or contains, one of the existing ranges.\"}",
				http.StatusUnprocessableEntity)
			return
		}
		// Insert
		_, err = vars.db.Exec("INSERT INTO ipranges(iprange,comment,use_dns) "+
			"VALUES($1,$2,$3)", iprange, req.FormValue("comment"),
			req.FormValue("useDns") != "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "", http.StatusCreated) // 201 Created
	case "DELETE":
		match := regexp.MustCompile("/(\\d+)$").FindStringSubmatch(req.URL.Path)
		if match == nil {
			http.Error(w, "Missing ipRangeId in URL path", http.StatusUnprocessableEntity)
			return
		}
		ipRangeID, _ := strconv.Atoi(match[1])
		_, err := vars.db.Exec("DELETE FROM ipranges WHERE iprangeid=$1", ipRangeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "", http.StatusNoContent) // 204 OK
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
