package main

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/lib/pq"
)

type apiMethodHost struct {
	db *sql.DB
}

func (vars *apiMethodHost) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case httpGET:
		(*vars).serveGET(w, req)
	case httpDELETE:
		(*vars).serveDELETE(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (vars *apiMethodHost) serveGET(w http.ResponseWriter, req *http.Request) {
	fields, hErr := unpackFieldParam(req.FormValue("fields"),
		[]string{"ipAddress", "hostname", "lastseen", "os", "osEdition",
			"kernel", "manufacturer", "product", "serialNo", "certfp",
			"clientVersion", "files", "support"})
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	qparams := make([]interface{}, 0)
	statement := "SELECT ipaddr, hostname, lastseen, os, os_edition, " +
		"kernel, manufacturer, product, serialno, certfp, clientversion " +
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
		var ipaddr, hostname, os, osEdition, kernel, manufacturer,
			product, serialNo, certfp, clientversion sql.NullString
		var lastseen pq.NullTime
		err = rows.Scan(&ipaddr, &hostname, &lastseen, &os, &osEdition,
			&kernel, &manufacturer, &product, &serialNo, &certfp, &clientversion)
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
		if fields["manufacturer"] {
			res["manufacturer"] = jsonString(manufacturer)
		}
		if fields["product"] {
			res["product"] = jsonString(product)
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
	IsDeleted    bool       `json:"isDeleted"`
	LastModified jsonTime   `json:"lastModified"`
}

func makeFileList(db *sql.DB, certfp string) ([]apiFile, *httpError) {
	rows, err := db.Query("SELECT filename,is_command,current,mtime "+
		"FROM (SELECT filename,is_command,current,mtime,row_number() "+
		"OVER (PARTITION BY filename ORDER BY current DESC, mtime DESC) "+
		"FROM files WHERE certfp=$1) AS foo WHERE row_number=1 "+
		"ORDER BY filename",
		certfp)
	if err != nil {
		return nil, &httpError{code: http.StatusInternalServerError, message: err.Error()}
	}
	defer rows.Close()
	files := make([]apiFile, 0)
	for rows.Next() {
		var filename sql.NullString
		var isCommand, current sql.NullBool
		var mtime pq.NullTime
		err = rows.Scan(&filename, &isCommand, &current, &mtime)
		if err != nil {
			return nil, &httpError{code: http.StatusInternalServerError, message: err.Error()}
		}
		files = append(files, apiFile{
			Filename:     jsonString(filename),
			IsCommand:    isCommand.Bool,
			IsDeleted:    !current.Bool,
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

func (vars *apiMethodHost) serveDELETE(w http.ResponseWriter, req *http.Request) {
	certfp := req.FormValue("certfp")
	hostname := req.FormValue("hostname")
	if certfp == "" && hostname == "" {
		http.Error(w, "Missing a hostname or certfp parameter", http.StatusUnprocessableEntity)
		return
	}
	var tx *sql.Tx
	var err error
	defer func() {
		if r := recover(); r != nil {
			if tx != nil {
				tx.Rollback()
			}
			panic(r)
		} else if err != nil {
			if tx != nil {
				tx.Rollback()
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Println(err)
		}
	}()
	tx, err = vars.db.Begin()
	if err != nil {
		return
	}
	if certfp == "" {
		var nullstr sql.NullString
		err = tx.QueryRow("SELECT certfp FROM hostinfo WHERE hostname=$1",
			hostname).Scan(&nullstr)
		if err != nil {
			return
		}
		certfp = nullstr.String
	}
	if certfp == "" {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	_, err = tx.Exec("UPDATE files SET current=false WHERE certfp=$1", certfp)
	if err != nil {
		return
	}
	res, err := tx.Exec("DELETE FROM hostinfo WHERE certfp=$1", certfp)
	if err != nil {
		return
	}
	rowcount, err := res.RowsAffected()
	if err != nil {
		return
	}
	err = tx.Commit()
	if err != nil {
		return
	}
	if rowcount == 0 {
		http.Error(w, "Host not found", http.StatusNotFound) // 404
	} else {
		http.Error(w, "", http.StatusNoContent) // 204 OK
	}
}
