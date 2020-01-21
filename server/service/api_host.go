package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/lib/pq"
	"github.com/unioslo/nivlheim/server/service/utility"
)

type apiMethodHost struct {
	db *sql.DB
}

func (vars *apiMethodHost) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	switch req.Method {
	case httpGET:
		(*vars).serveGET(w, req, access)
	case httpDELETE:
		(*vars).serveDELETE(w, req, access)
	case httpPATCH:
		(*vars).servePATCH(w, req, access)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (vars *apiMethodHost) serveGET(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// Get the certificate fingerprint (possibly based on a hostname) from the URL path
	certfp, err := getHostFromURLPath(req.URL.Path, vars.db)
	if err != nil {
		if he, ok := err.(*httpError); ok {
			http.Error(w, he.message, he.code)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Get the owner group
	var ownerGroup sql.NullString
	err = vars.db.QueryRow("SELECT ownergroup FROM hostinfo WHERE certfp=$1", certfp).Scan(&ownerGroup)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Host not found.", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Check that the user has access to this host
	if !access.HasAccessToGroup(ownerGroup.String) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// Get a list of names and IDs of all defined custom fields
	customFields, customFieldIDs, err := getListOfCustomFields(vars.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Make a complete list of allowed field names (standard + custom)
	allowedFields := []string{"ipAddress", "hostname", "lastseen", "os", "osEdition",
		"osFamily", "kernel", "manufacturer", "product", "serialNo", "certfp",
		"clientVersion", "files", "support", "overrideHostname", "ownerGroup"}
	allowedFields = append(allowedFields, customFields...)

	// The "fields" parameter says which fields I am supposed to return
	fields, hErr := unpackFieldParam(req.FormValue("fields"), allowedFields)
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	// Make a sql statement.
	statement := "SELECT ipaddr, COALESCE(hostname,host(ipaddr)) as hostname, lastseen, os, os_edition, " +
		"os_family, kernel, manufacturer, product, serialno, clientversion, override_hostname " +
		"FROM hostinfo WHERE certfp=$1"

	// Query the database for one row from the hostinfo table
	var ipaddr, hostname, os, osEdition, osFamily, kernel, manufacturer,
		product, serialNo, clientversion, overrideHostname sql.NullString
	var lastseen pq.NullTime
	err = vars.db.QueryRow(statement, certfp).
		Scan(&ipaddr, &hostname, &lastseen, &os, &osEdition, &osFamily,
			&kernel, &manufacturer, &product, &serialNo, &clientversion,
			&overrideHostname)
	if err == sql.ErrNoRows {
		// No host found. Return a "not found" status
		http.Error(w, "Host not found.", http.StatusNotFound)
		return
	}
	if err != nil {
		// SQL/database error
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Pick out values for the result
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
	if fields["osFamily"] {
		res["osFamily"] = jsonString(osFamily)
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
		res["certfp"] = certfp
	}
	if fields["clientVersion"] {
		res["clientVersion"] = jsonString(clientversion)
	}
	if fields["overrideHostname"] {
		res["overrideHostname"] = jsonString(overrideHostname)
	}
	if fields["ownerGroup"] {
		res["ownerGroup"] = jsonString(ownerGroup)
	}
	if fields["files"] {
		files, err := makeFileList(vars.db, certfp)
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
	// add the custom fields to the result
	for _, name := range customFields {
		if fields[name] {
			var value sql.NullString
			err = vars.db.QueryRow(
				"SELECT value FROM hostinfo_customfields "+
					"WHERE certfp=$1 AND fieldid=$2",
				certfp, customFieldIDs[name]).Scan(&value)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			res[name] = jsonString(value)
		}
	}
	returnJSON(w, req, res)
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

func (vars *apiMethodHost) serveDELETE(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// Get the certificate fingerprint (possibly based on a hostname) from the URL path
	certfp, err := getHostFromURLPath(req.URL.Path, vars.db)
	if err != nil {
		if he, ok := err.(*httpError); ok {
			http.Error(w, he.message, he.code)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Get the owner group
	var ownerGroup sql.NullString
	err = vars.db.QueryRow("SELECT ownergroup FROM hostinfo WHERE certfp=$1", certfp).Scan(&ownerGroup)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Host not found.", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Check that the user has access to this host
	if !access.HasAccessToGroup(ownerGroup.String) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// Run the whole operation in a transaction
	err = utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {
		// Mark all files from this certificate as "not current"
		_, err = tx.Exec("UPDATE files SET current=false WHERE certfp=$1", certfp)
		if err != nil {
			return err
		}
		// Remove the machine from hostinfo
		res, err := tx.Exec("DELETE FROM hostinfo WHERE certfp=$1", certfp)
		if err != nil {
			return err
		}
		rowcount, err := res.RowsAffected()
		if err != nil {
			return err
		}
		// Remove all the files from the search cache
		removeHostFromFastSearch(certfp)
		// Return status
		if rowcount == 0 {
			http.Error(w, "Host not found", http.StatusNotFound) // 404
		} else {
			http.Error(w, "", http.StatusNoContent) // 204 OK
		}
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (vars *apiMethodHost) servePATCH(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// Get the certificate fingerprint (possibly based on a hostname) from the URL path
	certfp, err := getHostFromURLPath(req.URL.Path, vars.db)
	if err != nil {
		if he, ok := err.(*httpError); ok {
			http.Error(w, he.message, he.code)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Get the owner group
	var ownerGroup sql.NullString
	err = vars.db.QueryRow("SELECT ownergroup FROM hostinfo WHERE certfp=$1", certfp).Scan(&ownerGroup)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Host not found.", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Check that the user has access to this host
	if !access.HasAccessToGroup(ownerGroup.String) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// Run the whole operation in a transaction
	err = utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {
		// Get the new data values. Expect the body to be form/urlencoded, not json
		err := req.ParseForm()
		if err != nil && req.ContentLength > 0 {
			http.Error(w, fmt.Sprintf("Unable to parse the form data: %s", err.Error()),
				http.StatusBadRequest)
			return nil
		}
		s, ok := ifFormValue(req.PostForm, "overridehostname")
		if ok {
			var count int
			err := tx.QueryRow("SELECT count(*) FROM hostinfo"+
				" WHERE (hostname=$1 OR override_hostname=$1)"+
				" AND certfp!=$2", s, certfp).Scan(&count)
			if count > 0 {
				// The name is already in use
				http.Error(w, "The name is already in use.", http.StatusConflict)
				return nil
			}
			res, err := tx.Exec("UPDATE hostinfo SET override_hostname=$1,dnsttl=null WHERE certfp=$2",
				s, certfp)
			if err != nil {
				return err
			}
			rowcount, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if rowcount == 0 {
				http.Error(w, "Host not found", http.StatusNotFound) // 404
			} else {
				http.Error(w, "", http.StatusNoContent) // 204 OK
			}
		} else {
			// The request is correct but doesn't contain any data (that I recognize anyway)
			http.Error(w, "", http.StatusUnprocessableEntity) // 422
		}
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Get a list of names and IDs of all defined custom fields
func getListOfCustomFields(db *sql.DB) ([]string, map[string]int, error) {
	customFields := make([]string, 0)
	customFieldIDs := make(map[string]int)
	rows, err := db.Query("SELECT fieldid,name FROM customfields")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var fieldID int
		var name string
		err = rows.Scan(&fieldID, &name)
		if err != nil {
			return nil, nil, err
		}
		customFields = append(customFields, name)
		customFieldIDs[name] = fieldID
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}
	rows.Close()
	return customFields, customFieldIDs, nil
}

type rowQueryer interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

// getHostFromURLPath parses a URL path and picks out the
// hostname or certificate fingerprint at the end.
// If a hostname is given, it looks it up in the database
// and returns the corresponding certificate fingerprint.
func getHostFromURLPath(path string, db rowQueryer) (string, error) {
	fingerprintMatch := regexp.MustCompile("/host/([a-fA-F0-9]{32,40})$").FindStringSubmatch(path)
	if fingerprintMatch != nil {
		return fingerprintMatch[1], nil
	}
	hostnameMatch := regexp.MustCompile("/host/([\\w\\.\\-]+)$").FindStringSubmatch(path)
	if hostnameMatch != nil {
		var nullstr sql.NullString
		err := db.QueryRow("SELECT certfp FROM hostinfo WHERE hostname=$1",
			hostnameMatch[1]).Scan(&nullstr)
		if err == sql.ErrNoRows {
			return "", &httpError{code: http.StatusNotFound, message: "hostname not found"}
		}
		if err != nil {
			return "", &httpError{code: http.StatusInternalServerError, message: err.Error()}
		}
		return nullstr.String, nil
	}
	return "", &httpError{code: http.StatusUnprocessableEntity,
		message: "Missing hostname or certificate fingerprint in URL path"}
}
