package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
)

type apiMethodGrep struct {
	db *sql.DB
}

func (vars *apiMethodGrep) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	if req.Method != httpGET {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := req.FormValue("q")
	if query == "" {
		http.Error(w, "Missing or empty parameter: q", http.StatusUnprocessableEntity)
		return
	}

	var limit int
	limit, err := strconv.Atoi(req.FormValue("limit"))
	if err != nil {
		limit = math.MaxInt64
	}

	if !isReadyForSearch() {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Not ready yet, still loading data", http.StatusServiceUnavailable)
		return
	}

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

	w.Header().Set("Content-Type", "text/plain")

	// Grab hostnames from the database, they're not in memory
	var stmt string
	if config.HideUnknownHosts {
		stmt = "SELECT certfp,hostname FROM hostinfo WHERE hostname IS NOT NULL"
	} else {
		stmt = "SELECT certfp,COALESCE(hostname,host(ipaddr)) FROM hostinfo"
	}
	rows, err := vars.db.Query(stmt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	certfp2hostname := make(map[string]string, 100)
	defer rows.Close() // in case of unexpected errors
	for rows.Next() {
		var certfp, hostname sql.NullString
		err = rows.Scan(&certfp, &hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if certfp.Valid && hostname.Valid {
			certfp2hostname[certfp.String] = hostname.String
		}
	}
	rows.Close()

	// Prepare a statement to retrieve the content for a file
	pstmt, err := vars.db.Prepare("SELECT content FROM files WHERE fileid=$1")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer pstmt.Close()

	bw := bufio.NewWriter(w)
	lineCount := 0
outer:
	for _, fileID := range hitIDs {
		matches := findMatchesInFile(fileID, query, math.MaxInt64)
		certfp, filename := getCertAndFilenameFromFileID(fileID)
		previousStart := -1

		// Possibly filter out hosts with undetermined hostnames
		if config.HideUnknownHosts {
			if _, ok := certfp2hostname[certfp]; !ok {
				continue
			}
		}

		// Retrieve the file content from the database
		var nstr sql.NullString
		err := pstmt.QueryRow(fileID).Scan(&nstr)
		if err != nil && err != sql.ErrNoRows {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err == sql.ErrNoRows || !nstr.Valid {
			continue
		}
		content := nstr.String

		for _, index := range matches {
			// Find the start and end of the line where the match occurred
			start := index
			for ; start > 0 && content[start-1] != '\n'; start-- {
			}
			if start == previousStart {
				continue
			}
			previousStart = start
			end := index
			for ; end < len(content) && content[end] != '\n'; end++ {
			}
			// Write the whole line to the output
			fmt.Fprintf(bw, "%s:%s:%s\n", certfp2hostname[certfp], filename, content[start:end])
			lineCount++
			if lineCount >= limit {
				break outer
			}
		}
	}
	bw.Flush()
}
