package main

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/lib/pq"
)

type apiMethodFile struct {
	db *sql.DB
}

func (vars *apiMethodFile) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	fields, hErr := unpackFieldParam(req.FormValue("fields"),
		[]string{"fileId", "filename", "isCommand", "lastModified", "received",
			"content", "certfp", "hostname", "versions"})
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	if req.FormValue("fileId") == "" {
		http.Error(w, "Missing parameter: fileId", http.StatusUnprocessableEntity)
		return
	}
	fileid, err := strconv.Atoi(req.FormValue("fileId"))
	if err != nil {
		http.Error(w, "Unable to parse fileId", http.StatusBadRequest)
		return
	}

	//TODO fileId -> fileID ??  les google standard

	statement := "SELECT fileid,filename,is_command,mtime,received,content," +
		"certfp,hostname FROM files f " +
		"LEFT JOIN hostinfo h USING (certfp) " +
		"WHERE fileid=$1"
	rows, err := vars.db.Query(statement, fileid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	if rows.Next() {
		var fileID int
		var filename, content, certfp, hostname sql.NullString
		var isCommand sql.NullBool
		var mtime, rtime pq.NullTime
		err = rows.Scan(&fileID, &filename, &isCommand, &mtime, &rtime, &content,
			&certfp, &hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		res := make(map[string]interface{}, 0)
		if fields["fileid"] {
			res["fileId"] = fileID
		}
		if fields["filename"] {
			res["filename"] = filename.String
		}
		if fields["iscommand"] {
			res["isCommand"] = isCommand.Bool
		}
		if fields["lastmodified"] {
			res["lastModified"] = jsonTime(mtime.Time)
		}
		if fields["received"] {
			res["received"] = jsonTime(rtime.Time)
		}
		if fields["content"] {
			res["content"] = content.String
		}
		if fields["certfp"] {
			res["certfp"] = certfp.String
		}
		if fields["hostname"] {
			res["hostname"] = hostname.String
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
				FileID int      `json:"fileId"`
				Mtime  jsonTime `json:"lastModified"`
			}
			versions := make([]Version, 0)
			for rows2.Next() {
				err = rows2.Scan(&fileID, &mtime)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				v := Version{FileID: fileID, Mtime: jsonTime(mtime.Time)}
				versions = append(versions, v)
			}
			if err = rows2.Err(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			res["versions"] = versions
		}
		returnJSON(w, req, res)
	} else {
		if err = rows.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// No file found. Return a "not found" status instead
		http.Error(w, "File not found.", http.StatusNotFound)
	}
}
