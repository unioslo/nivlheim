package main

import (
	"database/sql"
	"net"
	"net/http"
	"strings"

	"github.com/lib/pq"
)

type apiMethodAwaitingApproval struct {
	db *sql.DB
}

func (vars *apiMethodAwaitingApproval) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	fields, hErr := unpackFieldParam(req.FormValue("fields"),
		[]string{"ipAddress", "reverseDns", "hostname", "received"})
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	rows, err := vars.db.Query("SELECT ipaddr, hostname, received " +
		"FROM waiting_for_approval WHERE approved IS NULL ORDER BY hostname")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var ipaddress, hostname sql.NullString
		var received pq.NullTime
		err = rows.Scan(&ipaddress, &hostname, &received)
		if err != nil && err != sql.ErrNoRows {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		item := make(map[string]interface{})
		if fields["ipaddress"] {
			item["ipAddress"] = jsonString(ipaddress)
		}
		if fields["hostname"] {
			item["hostname"] = jsonString(hostname)
		}
		if fields["received"] {
			item["received"] = jsonTime(received)
		}
		if fields["reversedns"] {
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
		result = append(result, item)
	}

	type Wrapper struct {
		A []map[string]interface{} `json:"awaitingApproval"`
	}
	returnJSON(w, req, Wrapper{A: result})
}
