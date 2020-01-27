package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

type apiMethodFile struct {
	db *sql.DB
}

func (vars *apiMethodFile) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	if req.Method != httpGET {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var fields map[string]bool
	var hErr *httpError

	// Output format: Just the raw contents, or a json structure with fields?
	var rawFormat = req.FormValue("format") == "raw"

	if rawFormat {
		fields = make(map[string]bool, 0)
		fields["content"] = true
		if req.FormValue("fields") != "" {
			http.Error(w, "Can't use format=raw with fields parameter", http.StatusBadRequest)
			return
		}
		if req.FormValue("certfp") == "" && req.FormValue("hostname") == "" && req.FormValue("fileId") == "" {
			http.Error(w, "Can't use format=raw without specifying a host", http.StatusBadRequest)
			return
		}
	} else {
		fields, hErr = unpackFieldParam(req.FormValue("fields"),
			[]string{"fileId", "filename", "isCommand", "lastModified", "received",
				"content", "certfp", "hostname", "versions",
				"isNewestVersion", "isDeleted"})
		if hErr != nil {
			http.Error(w, hErr.message, hErr.code)
			return
		}
	}

	// Here's the SELECT/FROM part of the SQL statement
	statement := "SELECT fileid,filename,is_command,mtime,received,content," +
		"certfp,h.ownergroup,COALESCE(h.hostname,host(h.ipaddr)),current FROM files f " +
		"LEFT JOIN hostinfo h USING (certfp) "
	var rows *sql.Rows
	var err error
	var expectingOneRow = true

	if req.FormValue("fileId") != "" {
		// If a fileId is given, it tells us not only which file but which revision and from which host.
		var fileID int64
		fileID, err = strconv.ParseInt(req.FormValue("fileId"), 10, 64)
		if err != nil {
			http.Error(w, "Unable to parse fileId", http.StatusBadRequest)
			return
		}
		statement += "WHERE fileid=$1"
		rows, err = vars.db.Query(statement, fileID)
	} else if req.FormValue("filename") != "" {
		statement += "WHERE filename=$1"
		var params = []interface{}{req.FormValue("filename")}
		// A filename was given. There could be more than one revision, we shall select the latest.
		if req.FormValue("hostname") != "" {
			statement += " AND h.hostname=$2 ORDER BY mtime DESC LIMIT 1"
			params = append(params, req.FormValue("hostname"))
		} else if req.FormValue("certfp") != "" {
			statement += " AND certfp=$2 ORDER BY mtime DESC LIMIT 1"
			params = append(params, req.FormValue("certfp"))
		} else {
			// No host specified? Ok so you want this file from all the hosts.
			expectingOneRow = false
			// Filter on current so you only get the current version of each file.
			statement += " AND current"
			// Filter out what you don't have access to
			if !access.HasAccessToAllGroups() {
				statement += " AND h.ownergroup IN (" + access.GetGroupListForSQLWHERE() + ")"
			}
			// Any requirement for lastseen?
			matches := regexp.MustCompile(`lastseen([<>=])(\d+)([smhd])`).
				FindStringSubmatch(req.URL.RawQuery)
			if matches != nil {
				operator := matches[1]
				count, _ := strconv.Atoi(matches[2])
				unit := matches[3]
				statement += fmt.Sprintf(" AND now()-interval '%d%s' %s lastseen",
					count, unit, operator)
			} else if strings.Contains(req.URL.RawQuery, "lastseen") {
				http.Error(w, "Wrong format for lastseen parameter", http.StatusBadRequest)
				return
			}
		}
		rows, err = vars.db.Query(statement, params...)
	}

	if rows == nil && err == nil {
		http.Error(w, "Missing parameters. Requires either fileId or (filename + hostname/certfp)",
			http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var fileID int64
		var filename, content, certfp, ownerGroup, hostname sql.NullString
		var isCommand, isCurrent sql.NullBool
		var mtime, rtime pq.NullTime
		err = rows.Scan(&fileID, &filename, &isCommand, &mtime, &rtime, &content,
			&certfp, &ownerGroup, &hostname, &isCurrent)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !access.HasAccessToGroup(ownerGroup.String) {
			http.Error(w, "You don't have access to that resource.", http.StatusForbidden)
			return
		}

		res := make(map[string]interface{}, 0)
		if fields["fileId"] {
			res["fileId"] = fileID
		}
		if fields["filename"] {
			res["filename"] = jsonString(filename)
		}
		if fields["isCommand"] {
			res["isCommand"] = isCommand.Bool
		}
		if fields["isNewestVersion"] {
			res["isNewestVersion"] = isCurrent.Bool
		}
		if fields["isDeleted"] {
			var count int
			vars.db.QueryRow("SELECT count(*) FROM files WHERE current "+
				"AND certfp=$1 AND filename=$2", certfp, filename).Scan(&count)
			res["isDeleted"] = count == 0
		}
		if fields["lastModified"] {
			res["lastModified"] = jsonTime(mtime)
		}
		if fields["received"] {
			res["received"] = jsonTime(rtime)
		}
		if fields["content"] {
			res["content"] = jsonString(content)
		}
		if fields["certfp"] {
			res["certfp"] = jsonString(certfp)
		}
		if fields["hostname"] {
			res["hostname"] = jsonString(hostname)
		}
		if fields["versions"] {
			var rows2 *sql.Rows
			rows2, err = vars.db.Query("SELECT fileid,mtime FROM files "+
				"WHERE filename=$1 AND certfp=$2 ORDER BY mtime DESC",
				filename, certfp)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows2.Close()
			type Version struct {
				FileID int64    `json:"fileId"`
				Mtime  jsonTime `json:"lastModified"`
			}
			versions := make([]Version, 0)
			for rows2.Next() {
				err = rows2.Scan(&fileID, &mtime)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				v := Version{FileID: fileID, Mtime: jsonTime(mtime)}
				versions = append(versions, v)
			}
			if err = rows2.Err(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			res["versions"] = versions
		}
		result = append(result, res)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if expectingOneRow {
		if len(result) == 0 {
			// No file found. Return a "not found" status instead
			http.Error(w, "File not found.", http.StatusNotFound)
			return
		}
		if rawFormat {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			str := result[0]["content"].(jsonString)
			w.Write([]byte(str.String))
			return
		}
		returnJSON(w, req, result[0])
	} else {
		returnJSON(w, req, result)
	}
}
