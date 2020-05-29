package main

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type apiMethodSearch struct {
	db *sql.DB
}

func (vars *apiMethodSearch) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	if req.Method != httpGET {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := req.FormValue("q")
	if query == "" {
		http.Error(w, "Missing or empty parameter: q", http.StatusBadRequest)
		return
	}

	// Get a list of names and IDs of all defined custom fields
	customFields, customFieldIDs, err := getListOfCustomFields(vars.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Define which fields that you can ask for from the file table
	fileTableFields := []string{"fileID", "filename", "content"}

	// Make a complete list of allowed field names
	// (Fields from the host (including custom fields) + some from the file)
	allowedFields := make([]string, len(apiHostListStandardFields))
	for i, f := range apiHostListStandardFields {
		allowedFields[i] = f.publicName
	}
	allowedFields = append(allowedFields, customFields...)
	allowedFields = append(allowedFields, fileTableFields...)

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

	// Perform the search
	filename := req.FormValue("filename")
	var hitIDs []int64
	if access.HasAccessToAllGroups() {
		hitIDs, _ = searchFiles(query, filename)
	} else {
		// Compute a list of which certificates the user has access to,
		// based on current hosts in hostinfo owned by one of the groups the user has access to.
		list, err := QueryColumn(vars.db, "SELECT certfp FROM hostinfo WHERE ownergroup IN ("+
			access.GetGroupListForSQLWHERE()+")")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Println(err.Error())
			return
		}
		// List is a slice of interface{}, so I must convert that to a map[string]bool
		validCerts := make(map[string]bool, 100)
		for _, s := range list {
			str, ok := s.(string)
			if ok {
				validCerts[str] = true
			}
		}
		// Finally, we can perform the search
		hitIDs, _ = searchFilesWithFilter(query, filename, validCerts)
	}

	// We probably need to read additional information from the database,
	// unless the user only asked for the fileID of each file.
	var selectedFields []string
	var prepStatmt *sql.Stmt
	if !(len(fields) == 1 && fields["fileID"]) {

		// Build the "SELECT ... " part of the statement.
		// Start with the host fields:
		temp := make([]string, 0, len(fields))
		for _, f := range apiHostListStandardFields {
			if fields[f.publicName] {
				if f.publicName == "hostname" {
					temp = append(temp, "COALESCE(hostname,host(h.ipaddr))")
				} else if f.publicName == "ipAddress" {
					temp = append(temp, "host(h.ipaddr)")
				} else {
					temp = append(temp, "h."+f.columnName)
				}
				selectedFields = append(selectedFields, f.publicName)
			}
		}

		// Then, file fields:
		for _, name := range fileTableFields {
			if fields[name] && name != "fileID" {
				temp = append(temp, "f."+name)
				selectedFields = append(selectedFields, name)
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
		statement := "SELECT " + strings.Join(temp, ",") + " FROM files f, hostinfo h " +
					"WHERE f.certfp=h.certfp AND f.fileID=$1"
		// Possibly filter out hosts with undetermined hostnames
		if config.HideUnknownHosts {
			statement += " AND h.hostname IS NOT NULL"
		}
		prepStatmt, err = vars.db.Prepare(statement)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer prepStatmt.Close()
	}

	// Possibly filter out hosts with undetermined hostnames,
	// if it hasn't already been done by the code above
	if config.HideUnknownHosts && prepStatmt == nil {
		prepStatmt, err = vars.db.Prepare(
			"SELECT fileid FROM files f, hostinfo h WHERE f.certfp=h.certfp "+
			"AND f.fileID=$1 AND h.hostname IS NOT NULL")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer prepStatmt.Close()
	}

	// Data structure to return as JSON
	result := make([]map[string]interface{}, 0, len(hitIDs))

	// Temporary variable to hold result from QueryRow.Scan
	scanvars := make([]sql.NullString, len(selectedFields))
	scanpointers := make([]interface{}, len(scanvars))
	for i := 0; i < len(scanvars); i++ {
		scanpointers[i] = &scanvars[i]
	}

	// For each file found in the search, pick out the desired return fields and add them to a data structure.
	for _, fileID := range hitIDs {
		fileDetails := make(map[string]interface{}, len(fields))
		if fields["fileID"] {
			fileDetails["fileID"] = fileID
		}
		if prepStatmt != nil {
			err = prepStatmt.QueryRow(fileID).Scan(scanpointers...)
			if err != nil && err != sql.ErrNoRows {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err == sql.ErrNoRows {
				continue
			}
			for i, name := range selectedFields {
				fileDetails[name] = jsonString(scanvars[i])
			}
		}
		result = append(result, fileDetails)
	}

	// Return everything as JSON
	returnJSON(w, req, result)
}
