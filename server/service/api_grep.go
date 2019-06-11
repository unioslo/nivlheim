package main

import (
	"bufio"
	"database/sql"
	"fmt"
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

	var hitIDs []int64
	if access.IsAdmin() {
		hitIDs = searchFiles(query)
	} else {
		hitIDs = searchFilesWithFilter(query, access)
	}

	w.Header().Set("Content-Type", "text/plain")

	// Grab hostnames from the database, they're not in memory
	rows, err := vars.db.Query("SELECT certfp,COALESCE(hostname,host(ipaddr)) FROM hostinfo")
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

	bw := bufio.NewWriter(w)
	lineCount := 0
outer:
	for _, fileID := range hitIDs {
		matches := findMatchesInFile(fileID, query, math.MaxInt64)
		certfp, filename, content := getFileFromCache(fileID)
		previousStart := -1
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
