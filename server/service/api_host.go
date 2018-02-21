package main

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/lib/pq"
)

type apiMethodHost struct {
	db *sql.DB
}

func (vars *apiMethodHost) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	fields, hErr := unpackFieldParam(req.FormValue("fields"),
		[]string{"ipAddress", "hostname", "lastseen", "os", "osEdition",
			"kernel", "vendor", "model", "serialNo", "certfp",
			"clientVersion", "files", "support"})
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	qparams := make([]interface{}, 0)
	statement := "SELECT ipaddr, hostname, lastseen, os, os_edition, " +
		"kernel, vendor, model, serialno, certfp, clientversion " +
		"FROM hostinfo "
	if req.FormValue("hostname") != "" {
		statement += "WHERE hostname=$1"
		qparams = append(qparams, req.FormValue("hostname"))
	} else if req.FormValue("certfp") != "" {
		statement += "WHERE certfp=$1"
		qparams = append(qparams, req.FormValue("certfp"))
	} else {
		http.Error(w, "Missing parameters. Requires either hostname or certfp.",
			http.StatusUnprocessableEntity)
		return
	}

	rows, err := vars.db.Query(statement, qparams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	if rows.Next() {
		var ipaddr, hostname, os, osEdition, kernel, vendor,
			model, serialNo, certfp, clientversion sql.NullString
		var lastseen pq.NullTime
		err = rows.Scan(&ipaddr, &hostname, &lastseen, &os, &osEdition,
			&kernel, &vendor, &model, &serialNo, &certfp, &clientversion)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		res := make(map[string]interface{}, 0)
		if fields["ipAddress"] {
			res["ipAddress"] = jsonString(ipaddr)
		}
		if fields["hostname"] {
			res["hostname"] = jsonString(hostname)
		}
		if fields["lastseen"] {
			res["lastseen"] = jsonTime(lastseen)
		}
		if fields["os"] {
			res["os"] = jsonString(os)
		}
		if fields["osEdition"] {
			res["osEdition"] = jsonString(osEdition)
		}
		if fields["kernel"] {
			res["kernel"] = jsonString(kernel)
		}
		if fields["vendor"] {
			res["vendor"] = jsonString(vendor)
		}
		if fields["model"] {
			res["model"] = jsonString(model)
		}
		if fields["serialNo"] {
			res["serialNo"] = jsonString(serialNo)
		}
		if fields["certfp"] {
			res["certfp"] = jsonString(certfp)
		}
		if fields["clientVersion"] {
			res["clientVersion"] = jsonString(clientversion)
		}
		if fields["files"] {
			files, err := makeFileList(vars.db, certfp.String)
			if err != nil {
				http.Error(w, err.message, err.code)
				return
			}
			res["files"] = files
		}
		if fields["support"] {
			support, err := makeSupportList(vars.db, serialNo.String)
			if err != nil {
				http.Error(w, err.message, err.code)
				return
			}
			res["support"] = support
		}
		returnJSON(w, req, res)
	} else {
		if err = rows.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// No host found. Return a "not found" status instead
		http.Error(w, "Host not found.", http.StatusNotFound)
	}
}

type apiFile struct {
	Filename     jsonString `json:"filename"`
	IsCommand    bool       `json:"isCommand"`
	LastModified jsonTime   `json:"lastModified"`
}

func makeFileList(db *sql.DB, certfp string) ([]apiFile, *httpError) {
	rows, err := db.Query("SELECT filename,is_command,max(mtime) FROM files "+
		"WHERE certfp=$1 GROUP BY filename, is_command ORDER BY filename",
		certfp)
	if err != nil {
		return nil, &httpError{code: http.StatusInternalServerError, message: err.Error()}
	}
	defer rows.Close()
	files := make([]apiFile, 0)
	for rows.Next() {
		var filename sql.NullString
		var isCommand sql.NullBool
		var mtime pq.NullTime
		err = rows.Scan(&filename, &isCommand, &mtime)
		if err != nil {
			return nil, &httpError{code: http.StatusInternalServerError, message: err.Error()}
		}
		files = append(files, apiFile{
			Filename:     jsonString(filename),
			IsCommand:    isCommand.Bool,
			LastModified: jsonTime(mtime),
		})
	}
	if err = rows.Err(); err != nil {
		return nil, &httpError{code: http.StatusInternalServerError, message: err.Error()}
	}
	return files, nil
}

type apiSupport struct {
	Start       jsonTime   `json:"start"`
	Expires     jsonTime   `json:"expires"`
	HasExpired  bool       `json:"hasExpired"`
	Description jsonString `json:"description"`
}

func makeSupportList(db *sql.DB, serialNo string) ([]apiSupport, *httpError) {
	rows, err := db.Query("SELECT start, expires, description FROM support "+
		"WHERE serialno=$1", serialNo)
	if err != nil {
		return nil, &httpError{code: http.StatusInternalServerError, message: err.Error()}
	}
	defer rows.Close()
	supportList := make([]apiSupport, 0)
	for rows.Next() {
		var start, exp pq.NullTime
		var desc sql.NullString
		rows.Scan(&start, &exp, &desc)
		supportList = append(supportList, apiSupport{
			Start:       jsonTime(start),
			Expires:     jsonTime(exp),
			HasExpired:  exp.Valid && exp.Time.Before(time.Now()),
			Description: jsonString(desc),
		})
	}
	if err = rows.Err(); err != nil {
		return nil, &httpError{code: http.StatusInternalServerError, message: err.Error()}
	}
	return supportList, nil
}
