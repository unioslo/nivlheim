package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type apiMethodMultiStageSearch struct {
	db *sql.DB
}

func (vars *apiMethodMultiStageSearch) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	if req.Method != httpGET {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get a list of names and IDs of all defined custom fields
	customFields, customFieldIDs, err := getListOfCustomFields(vars.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Make a complete list of allowed field names
	// (Fields from the host (including custom fields) + some from the file)
	allowedFields := make([]string, len(apiHostListStandardFields))
	for i, f := range apiHostListStandardFields {
		allowedFields[i] = f.publicName
	}
	allowedFields = append(allowedFields, customFields...)

	// Parse the "fields" parameter and verify that all given field names are valid
	fields, hErr := unpackFieldParam(req.FormValue("fields"), allowedFields)
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	// When the system service starts up, it can take a few seconds before the cache is loaded.
	// If we allowed search during this period, it would yield incomplete results.
	if !isReadyForSearch() {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Not ready yet, still loading data", http.StatusServiceUnavailable)
		return
	}

	// The user can specify a search that goes over several stages.
	// For more information, see https://github.com/unioslo/nivlheim/issues/121
	var resultingCerts = make(map[string]bool, 0)
	for stage := 1; ; stage++ {

		// Parse the "q" and "f" parameters for this stage.
		// Filename can be empty.
		var query, filename, operation string
		query = req.FormValue(fmt.Sprintf("q%d", stage))
		filename = req.FormValue(fmt.Sprintf("f%d", stage))
		operation = req.FormValue(fmt.Sprintf("op%d", stage))

		if query == "" {
			if stage == 1 {
				http.Error(w, "Missing or empty parameter: q1",
					http.StatusBadRequest)
				return
			}
			// Otherwise, I guess we're done
			break
		}

		// All stages after the first requires an operation
		if stage > 1 && operation == "" {
			http.Error(w, fmt.Sprintf("Missing or empty parameter: op%d", stage), http.StatusBadRequest)
			return
		}

		// Perform the search
		var hitIDs map[string]bool
		hitIDs = searchForHosts(query, filename) // If filename is empty, it searches all the files.

		if stage > 1 {
			// Perform the operation
			switch strings.ToUpper(operation) {
			case "AND": // intersection
				// remove entries from before that weren't in this stage's results:
				for id := range resultingCerts {
					if !hitIDs[id] {
						delete(resultingCerts, id)
					}
				}
			case "OR": // union
				for id := range hitIDs {
					resultingCerts[id] = true
				}
			case "SUB": // difference
				// Subtract this stage's results from the previous list
				for id := range hitIDs {
					delete(resultingCerts, id)
				}
			default:
				http.Error(w, fmt.Sprintf("Unsupported operation: %s", operation), http.StatusBadRequest)
				return
			}
		} else {
			// This is the first (and perhaps only) stage.
			// Copy the hitIDs list into the resultingFileIDs map.
			for id := range hitIDs {
				resultingCerts[id] = true
			}
		}
	}

	// Access control
	if !access.HasAccessToAllGroups() {
		// Compute a list of which certificates the user has access to,
		// based on current hosts in hostinfo owned by one of the groups the user has access to.
		list, err := QueryColumn(vars.db, "SELECT certfp FROM hostinfo WHERE ownergroup IN ("+
			access.GetGroupListForSQLWHERE()+")")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Println(err.Error())
			return
		}

		// Make a new hitList with only the hosts you have access to
		newMap := make(map[string]bool, len(resultingCerts))
		for _, s := range list {
			str, ok := s.(string)
			if ok && resultingCerts[str] {
				newMap[str] = true
			}
		}

		// Replace
		resultingCerts = newMap
	}

	// We probably need to read additional information from the database,
	// unless the user only asked for the certificate fingerprint from each host.
	var selectedFields []string
	var prepStatmt *sql.Stmt
	if !(len(fields) == 1 && fields["certfp"]) {

		// Build the "SELECT ... " part of the statement.
		// Start with the standard host fields:
		temp := make([]string, 0, len(fields))
		for _, f := range apiHostListStandardFields {
			if fields[f.publicName] {
				if f.publicName == "hostname" {
					temp = append(temp, "COALESCE(hostname,host(ipaddr))")
				} else if f.publicName == "ipAddress" {
					temp = append(temp, "host(ipaddr)")
				} else {
					temp = append(temp, f.columnName)
				}
				selectedFields = append(selectedFields, f.publicName)
			}
		}

		// Then, append any custom fields
		for _, name := range customFields {
			if fields[name] {
				temp = append(temp, "(SELECT value FROM hostinfo_customfields hc "+
					"WHERE hc.certfp=h.certfp AND hc.fieldid="+
					strconv.Itoa(customFieldIDs[name])+") as "+name)
				selectedFields = append(selectedFields, name)
			}
		}

		// Prepare SQL statement
		prepStatmt, err = vars.db.Prepare(
			"SELECT " + strings.Join(temp, ",") + " FROM hostinfo " +
				"WHERE certfp=$1")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer prepStatmt.Close()
	}

	// Data structure to return as JSON
	result := make([]map[string]interface{}, 0, len(resultingCerts))

	// Temporary variable to hold result from QueryRow.Scan
	scanvars := make([]sql.NullString, len(selectedFields))
	scanpointers := make([]interface{}, len(scanvars))
	for i := 0; i < len(scanvars); i++ {
		scanpointers[i] = &scanvars[i]
	}

	// Sort by certificate, to make unit testing easier
	list := make([]string, 0, len(resultingCerts))
	for certFP, isThere := range resultingCerts {
		// The map shouldn't contain entries where the value is false, but just in case...
		if !isThere {
			continue
		}
		list = append(list, certFP)
	}
	sort.Strings(list)

	// For each host found in the search, pick out the desired return fields and add them to a data structure.
	for _, certFP := range list {
		details := make(map[string]interface{}, len(fields))
		if fields["certfp"] {
			details["certfp"] = certFP
		}
		if prepStatmt != nil {
			err = prepStatmt.QueryRow(certFP).Scan(scanpointers...)
			if err != nil && err != sql.ErrNoRows {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err == sql.ErrNoRows {
				continue
			}
			for i, name := range selectedFields {
				details[name] = jsonString(scanvars[i])
			}
		}
		result = append(result, details)
	}

	// Return everything as JSON
	returnJSON(w, req, result)
}
