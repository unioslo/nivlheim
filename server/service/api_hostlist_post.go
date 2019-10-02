package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

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
	"ownerGroup":   "ownergroup",
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

	updated, created, errorCount := 0, 0, 0

	// Process the entries
	hash := fnv.New128a()
	for _, entry := range postdata {
		hostname, ok := entry["hostname"].(string)
		if !ok {
			// If no hostname was given, just skip that entry and continue down the list.
			continue
		}

		// Run operations for each host in a transaction to prevent data inconsistency in case of error,
		// and to be able to continue afterwards.
		err = utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {

			// If createIfNotExists, the field ownerGroup is required.
			// Can't create a new host without knowing which group owns it.
			if b, ok := entry["createIfNotExists"].(bool); b && ok {
				owner, ok2 := entry["ownerGroup"].(string)
				if !ok2 || strings.TrimSpace(owner) == "" {
					return &httpError{
						message: "The field ownerGroup is required for hosts when createIfNotExists is specified",
						code:    http.StatusBadRequest,
					}
				}
			}

			// Verify access to ownerGroup, if it was supplied
			owner, ok2 := entry["ownerGroup"].(string)
			if ok2 && !access.HasAccessToGroup(owner) {
				return &httpError{
					message: "You don't have access to the group " + owner,
					code:    http.StatusForbidden,
				}
			}

			// You're not allowed to update hosts you don't own.
			// If the host exists already, verify that you have access to it
			var owner2 sql.NullString
			err = tx.QueryRow("SELECT ownerGroup FROM hostinfo WHERE hostname=$1", hostname).Scan(&owner2)
			if err != nil && err != sql.ErrNoRows {
				return err
			} else if err != sql.ErrNoRows {
				if owner2.Valid && !access.HasAccessToGroup(owner2.String) {
					return &httpError{
						message: fmt.Sprintf("You don't have access to %s, it is owned by %s",
							hostname, owner2.String),
						code: http.StatusForbidden,
					}
				}
			}

			// Put together some data for the SQL statements
			columnValues := make(map[string]interface{})
			for publicName, columnName := range apiHostListWritableFields {
				value, ok := entry[publicName]
				if ok {
					columnValues[columnName] = value
				}
			}
			columnValues["lastseen"] = time.Now()

			// Update the host, if it exists
			sql, params := utility.BuildUpdateStatement("hostinfo", columnValues, "hostname", hostname)
			var rowsAffected int64
			res, err := tx.Exec(sql, params...)
			if err == nil {
				rowsAffected, err = res.RowsAffected()
			}
			if err != nil {
				log.Printf("hostlist_post error: %s: %s", err.Error(), sql)
				return err
			}
			if rowsAffected > 0 {
				updated++
			} else {
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
					_, err = tx.Exec(sql, params...)
					if err != nil {
						log.Printf("hostlist_post error: %s: %s", err.Error(), sql)
						return err
					}
					created++
				} else {
					// The host doesn't exist and I'm not supposed to create it.
					// Exit the transaction and go to the next host, then.
					return nil
				}
			}

			// handle any custom fields
			for _, m := range customFields {
				key := m["name"].(string)
				value, ok := entry[key]
				if ok {
					res, err := tx.Exec("UPDATE hostinfo_customfields SET value=$1 "+
						"WHERE fieldid=$2 AND certfp=(SELECT certfp FROM hostinfo WHERE hostname=$3)",
						value, m["fieldid"], hostname)
					var rowsAffected int64
					if err == nil {
						rowsAffected, err = res.RowsAffected()
					}
					if err != nil {
						log.Printf("hostlist post customfield error: %s", err.Error())
						return err
					}
					if rowsAffected == 0 {
						tx.Exec("INSERT INTO hostinfo_customfields(certfp,value,fieldid) "+
							"VALUES((SELECT certfp FROM hostinfo WHERE hostname=$1),$2, $3)",
							hostname, value, m["fieldid"])
					}
				}
			}
			return nil
		}) // End of transaction
		if err != nil {
			errorCount++
			var message string
			httpErr, ok := err.(*httpError)
			if ok {
				message = httpErr.message
			} else {
				message = err.Error()
			}
			fmt.Fprintf(w, "%s: %s\n", hostname, message)
		}
	}

	fmt.Fprintf(w, "Updated %d hosts, created %d new hosts, %d errors.\n", updated, created, errorCount)
}
