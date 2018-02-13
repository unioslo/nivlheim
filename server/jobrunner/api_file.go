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
			"content", "certfp", "hostname"})
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
		rows.Scan(&fileID, &filename, &isCommand, &mtime, &rtime, &content,
			&certfp, &hostname)
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
		returnJSON(w, req, res)
	} else {
		// No file found. Return a "not found" status instead
		http.Error(w, "File not found.", http.StatusNotFound)
	}
}
