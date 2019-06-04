package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/usit-gd/nivlheim/server/service/utility"
)

var apiHostListWritableFields = map[string]string{
	"ipAddress":    "ipaddr",
	"os":           "os",
	"osEdition":    "os_edition",
	"osFamily":     "os_family",
	"kernel":       "kernel",
	"manufacturer": "manufacturer",
	"product":      "product",
	"serialNo":     "serialno",
}

func (vars *apiMethodHostList) ServePOST(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// Read the request body
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Printf("Error reading request body: %s", err.Error())
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	// Decode the JSON
	var postdata = make([]map[string]interface{}, 0)
	err = json.Unmarshal(body, &postdata)
	if err != nil {
		msg := fmt.Sprintf("Error decoding JSON data: %s", err.Error())
		log.Println(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	// Read the names of defined custom fields from the database
	customFields, err := QueryList(vars.db, "SELECT fieldid,name FROM customfields")
	if err != nil {
		log.Println(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Process the entries
	updated, created := 0, 0
	hash := fnv.New128a()
	for _, entry := range postdata {
		hostname, ok := entry["hostname"].(string)
		if !ok {
			// If no hostname was given, just skip that entry and continue down the list.
			continue
		}

		// Put together some data for the SQL statements
		columnValues := make(map[string]interface{})
		for publicName, columnName := range apiHostListWritableFields {
			value, ok := entry[publicName]
			if ok {
				columnValues[columnName] = value
			}
		}

		if len(columnValues) > 0 {
			// Update the host, if it exists
			sql, params := utility.BuildUpdateStatement("hostinfo", columnValues, "hostname", hostname)
			var rowsAffected int64
			res, err := vars.db.Exec(sql, params...)
			if err == nil {
				rowsAffected, err = res.RowsAffected()
			}
			if err != nil {
				log.Printf("hostlist_post error: %s: %s", err.Error(), sql)
				http.Error(w, "Error while updating the database", http.StatusInternalServerError)
				return
			}
			if rowsAffected == 0 {
				// The host doesn't exist, perhaps create it?
				b, ok := entry["createIfNotExists"].(bool)
				if ok && b {
					// Must invent a value for the certificate fingerprint.
					// Use a hash of the hostname so it won't change
					hash.Reset()
					hash.Write([]byte(hostname))
					columnValues["certfp"] = fmt.Sprintf("%X", hash.Sum(nil))
					columnValues["hostname"] = hostname
					// Insert a row
					sql, params = utility.BuildInsertStatement("hostinfo", columnValues)
					_, err = vars.db.Exec(sql, params...)
					if err != nil {
						log.Printf("hostlist_post error: %s: %s", err.Error(), sql)
						http.Error(w, "Error while updating the database", http.StatusInternalServerError)
						return
					} else {
						created++
					}
				}
			} else {
				updated++
			}
		}

		// handle any custom fields
		for _, m := range customFields {
			key := m["name"].(string)
			value, ok := entry[key]
			if ok {
				res, err := vars.db.Exec("UPDATE hostinfo_customfields SET value=$1 "+
					"WHERE fieldid=$2 AND certfp=(SELECT certfp FROM hostinfo WHERE hostname=$3)",
					value, m["fieldid"], hostname)
				var rowsAffected int64
				if err == nil {
					rowsAffected, err = res.RowsAffected()
				}
				if err != nil {
					log.Printf("hostlist post customfield error: %s", err.Error())
					http.Error(w, "Error while updating the database", http.StatusInternalServerError)
					return
				}
				if rowsAffected == 0 {
					vars.db.Exec("INSERT INTO hostinfo_customfields(certfp,value,fieldid) "+
						"VALUES((SELECT certfp FROM hostinfo WHERE hostname=$1),$2, $3)",
						hostname, value, m["fieldid"])
				}
			}
		}
	}

	fmt.Fprintf(w, "Updated %d hosts, created %d new hosts\n", updated, created)
}
