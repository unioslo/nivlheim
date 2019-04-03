package main

import (
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

type apiMethodApproval struct {
	db *sql.DB
}

func (vars *apiMethodApproval) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case httpGET:
		vars.get(w, req)
	case httpPOST:
		vars.create(w, req)
	case httpPATCH:
		vars.partialUpdate(w, req)
	case httpDELETE:
		vars.delete(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (vars *apiMethodApproval) get(w http.ResponseWriter, req *http.Request) {
	// The fields parameter says which fields to include in the response
	fields, hErr := unpackFieldParam(req.FormValue("fields"),
		[]string{"ipAddress", "reverseDns", "hostname", "received", "approvalId", "approved"})
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	// Construct the query
	st := "SELECT ipaddr, hostname, received, approvalId, approved FROM waiting_for_approval"
	qparams := make([]interface{}, 0)
	if appr := req.FormValue("approved"); appr != "" {
		if appr == "null" {
			st += " WHERE approved IS null"
		} else {
			st += " WHERE approved = $1"
			qparams = append(qparams, isTrueish(appr))
		}
	}

	// Run the query
	rows, err := vars.db.Query(st, qparams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var approvalID int
		var ipaddress, hostname sql.NullString
		var received pq.NullTime
		var approved sql.NullBool
		err = rows.Scan(&ipaddress, &hostname, &received, &approvalID, &approved)
		if err != nil && err != sql.ErrNoRows {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		item := make(map[string]interface{})
		if fields["approvalId"] {
			item["approvalId"] = approvalID
		}
		if fields["ipAddress"] {
			item["ipAddress"] = jsonString(ipaddress)
		}
		if fields["hostname"] {
			item["hostname"] = jsonString(hostname)
		}
		if fields["received"] {
			item["received"] = jsonTime(received)
		}
		if fields["reverseDns"] {
			var r string
			if ipaddress.Valid {
				// Reverse DNS lookup
				names, err := net.LookupAddr(ipaddress.String)
				if err == nil && len(names) > 0 {
					r = strings.TrimRight(names[0], ".")
				}
			}
			item["reverseDns"] = r
		}
		if fields["approved"] {
			item["approved"] = jsonBool(approved)
		}
		result = append(result, item)
	}

	type Wrapper struct {
		A []map[string]interface{} `json:"manualApproval"`
	}
	returnJSON(w, req, Wrapper{A: result})
}

func (vars *apiMethodApproval) partialUpdate(w http.ResponseWriter, req *http.Request) {
	match := regexp.MustCompile("/(\\d+)$").FindStringSubmatch(req.URL.Path)
	if match == nil {
		http.Error(w, "Missing approvalId in URL path", http.StatusUnprocessableEntity)
		return
	}
	approvalID, _ := strconv.Atoi(match[1])

	var approved sql.NullBool
	if appr := req.FormValue("approved"); appr != "" {
		if appr != "null" {
			approved = sql.NullBool{Bool: isTrueish(appr), Valid: true}
		}
	} else {
		http.Error(w, "Missing required parameter: approved", http.StatusUnprocessableEntity)
		return
	}

	for key := range req.Form {
		if key != "approved" && key != "hostname" {
			http.Error(w, "Unsupported parameter: "+key, http.StatusBadRequest)
			return
		}
	}

	var hostname string
	var res sql.Result
	var err error
	if approved.Bool && approved.Valid {
		if hostname = req.FormValue("hostname"); hostname == "" {
			http.Error(w, "Missing required parameter: hostname", http.StatusUnprocessableEntity)
			return
		}
		var count int
		err = vars.db.QueryRow("SELECT count(*) FROM hostinfo WHERE hostname=$1 OR override_hostname=$1",
			hostname).Scan(&count)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if count > 0 {
			http.Error(w, "There's another machine with that hostname.",
				http.StatusConflict)
			return
		}
		res, err = vars.db.Exec("UPDATE waiting_for_approval SET approved=true, "+
			"hostname=$1 WHERE approvalId=$2", hostname, approvalID)
	} else {
		res, err = vars.db.Exec("UPDATE waiting_for_approval SET approved=$1 "+
			"WHERE approvalId=$2", approved, approvalID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rows, err := res.RowsAffected(); rows == 0 || err != nil {
		http.Error(w, fmt.Sprintf("Entity with id %d not found ", approvalID),
			http.StatusNotFound)
		return
	}
	http.Error(w, "", http.StatusNoContent) // 204 OK
}

func (vars *apiMethodApproval) create(w http.ResponseWriter, req *http.Request) {
	// parse the POST parameters
	err := req.ParseForm()
	if err != nil && req.ContentLength > 0 {
		http.Error(w, fmt.Sprintf("Unable to parse the form data: %s", err.Error()),
			http.StatusBadRequest)
		return
	}

	// check that required parameters are present
	hostname := formValue(req.PostForm, "hostname")
	if hostname == "" {
		http.Error(w, "hostname is missing or empty", http.StatusBadRequest)
		return
	}
	ipAddress := formValue(req.PostForm, "ipAddress")
	if ipAddress == "" {
		http.Error(w, "ipAddress is missing or empty", http.StatusBadRequest)
		return
	}
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		http.Error(w, "Malformed IP address: "+ipAddress, http.StatusBadRequest)
		return
	}
	var approved sql.NullBool
	if appr := req.FormValue("approved"); appr != "" {
		if appr != "null" {
			approved = sql.NullBool{Bool: isTrueish(appr), Valid: true}
		}
	} else {
		http.Error(w, "Missing required parameter: approved", http.StatusUnprocessableEntity)
		return
	}

	// check that the hostname isn't in use
	var count int
	err = vars.db.QueryRow("SELECT count(*) FROM hostinfo WHERE hostname=$1 OR override_hostname=$1",
		hostname).Scan(&count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, "There's another machine with that hostname.",
			http.StatusConflict)
		return
	}

	// insert a new row
	_, err = vars.db.Exec("INSERT INTO waiting_for_approval(hostname,ipaddr,received,approved)"+
		" VALUES($1,$2,now(),$3)", hostname, ipAddress, approved)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Error(w, "", http.StatusNoContent) // 204 OK
}

func (vars *apiMethodApproval) delete(w http.ResponseWriter, req *http.Request) {
	// Grab the item ID from the URL
	match := regexp.MustCompile("/(\\d+)$").FindStringSubmatch(req.URL.Path)
	if match == nil {
		http.Error(w, "Missing approvalId in URL path", http.StatusUnprocessableEntity)
		return
	}
	approvalID, _ := strconv.Atoi(match[1])
	// Delete the table row
	res, err := vars.db.Exec("DELETE FROM waiting_for_approval WHERE approvalId=$1",
		approvalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// If no rows were deleted, return a "not found" status
	if rows, err := res.RowsAffected(); rows == 0 || err != nil {
		http.Error(w, fmt.Sprintf("Entity with id %d not found ", approvalID),
			http.StatusNotFound)
		return
	}
	http.Error(w, "", http.StatusNoContent) // 204 OK
}
