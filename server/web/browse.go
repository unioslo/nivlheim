package main

import (
	"database/sql"
	"html/template"
	"net/http"
	"strconv"

	"github.com/lib/pq"
)

type Hostinfo struct {
	Hostname      sql.NullString
	IPaddr        sql.NullString
	Certfp        string
	Kernel        sql.NullString
	Type          sql.NullString
	Lastseen      pq.NullTime
	OS            sql.NullString
	OSclass       sql.NullString
	Vendor        sql.NullString
	Model         sql.NullString
	Serialno      sql.NullString
	Clientversion sql.NullString
}

type File struct {
	Fileid   int
	Filename sql.NullString
	Received pq.NullTime
	Content  sql.NullString
}

func browse(w http.ResponseWriter, req *http.Request) {
	db, err := sql.Open("postgres", dbConnectionString)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Load templates
	templates, err := template.ParseGlob(templatePath + "/*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tValues := make(map[string]interface{})
	var templatename string

	if (req.FormValue("f") != "" && req.FormValue("c") != "") ||
		req.FormValue("fid") != "" {

		templatename = "browsefile.html"
		fileid, err := strconv.Atoi(req.FormValue("fid"))
		if err != nil {
			err = db.QueryRow("SELECT fileid FROM files "+
				"WHERE filename=$1 AND certfp=$2 "+
				"ORDER BY received DESC LIMIT 1",
				req.FormValue("f"), req.FormValue("c")).Scan(&fileid)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if fileid == 0 {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		var f File
		var hostname sql.NullString
		err = db.QueryRow("SELECT fileid,content,filename,received,hostname "+
			"FROM files JOIN hostinfo ON hostinfo.certfp=files.certfp "+
			"WHERE fileid=$1", fileid).
			Scan(&f.Fileid, &f.Content, &f.Filename, &f.Received, &hostname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tValues["file"] = f
		if hostname.Valid {
			tValues["hostname"] = hostname.String
		}
	} else if req.FormValue("h") != "" {
		templatename = "browsehost.html"

		// Hostinfo
		var hi Hostinfo
		err = db.QueryRow("SELECT hostname, ipaddr, certfp, kernel, "+
			"type, lastseen, os, osclass, vendor, model, serialno, "+
			"clientversion FROM hostinfo WHERE hostname=$1",
			req.FormValue("h")).Scan(&hi.Hostname, &hi.IPaddr,
			&hi.Certfp, &hi.Kernel, &hi.Type, &hi.Lastseen,
			&hi.OS, &hi.OSclass, &hi.Vendor, &hi.Model,
			&hi.Serialno, &hi.Clientversion)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tValues["hostinfo"] = hi

		// File and command List
		files := make([]string, 0, 0)
		commands := make([]string, 0, 0)
		rows, err := db.Query("SELECT DISTINCT filename,is_command FROM files "+
			" WHERE certfp=$1 ORDER BY filename", &hi.Certfp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			defer rows.Close()
			for rows.Next() {
				var filename sql.NullString
				var isCommand sql.NullBool
				err = rows.Scan(&filename, &isCommand)
				if err != nil && err != sql.ErrNoRows {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if filename.Valid {
					if isCommand.Valid && isCommand.Bool {
						commands = append(commands, filename.String)
					} else {
						files = append(files, filename.String)
					}
				}
			}
		}
		tValues["files"] = files
		tValues["commands"] = commands
	} else {
		w.Write([]byte("Missing or wrong parameters."))
		return
	}

	// Render template
	err = templates.ExecuteTemplate(w, templatename, tValues)
	if err != nil {
		s := "<hr><p>\nTemplate: " + templatename + "<br>\n<pre>" + err.Error() + "</pre></p>"
		w.Write([]byte(s))
	}
}
