package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/usit-gd/nivlheim/server/service/utility"
)

type apiMethodKeys struct {
	db *sql.DB
}

func (vars *apiMethodKeys) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	switch req.Method {
	case httpGET:
		(*vars).read(w, req, access)
	case httpPOST:
		(*vars).create(w, req, access)
	case httpPUT:
		(*vars).update(w, req, access)
	case httpDELETE:
		(*vars).delete(w, req, access)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (vars *apiMethodKeys) read(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// See which fields I'm supposed to return
	fields, hErr := unpackFieldParam(req.FormValue("fields"), []string{
		"keyID", "key", "comment", "filter", "readonly", "expires", "ipRanges"})
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}
	columns := make([]string, len(fields))
	i := 0
	for k := range fields {
		if k == "ipRanges" {
			// ipranges isn't a column, but we'll need the "keyid" column in the SQL
			// to be able to select ipranges belonging to that key
			k = "keyID"
		}
		columns[i] = k
		i++
	}
	// read key with a specific id?
	if i := strings.LastIndex(req.URL.Path, "/keys/"); i > -1 {
		keyID, err := strconv.Atoi(req.URL.Path[i+6:])
		if err != nil {
			http.Error(w, "Invalid Key ID: "+req.URL.Path[i+6:], http.StatusBadRequest)
			return
		}
		data, err := QueryList(vars.db, "SELECT ownerid,"+strings.Join(columns, ",")+
			" FROM apikeys WHERE keyid=$1", keyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else if len(data) > 0 {
			if data[0]["ownerid"] != access.OwnerID() {
				http.Error(w, "This isn't your key.", http.StatusForbidden)
				return
			}
			delete(data[0], "ownerid")
			if fields["ipRanges"] {
				data[0]["ipRanges"], err = QueryColumn(vars.db, "SELECT iprange FROM apikey_ips WHERE keyid=$1", keyID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				// Remove the keyid column if they didn't ask for it
				if !fields["keyID"] {
					delete(data[0], "keyid")
				}
			}
			// keyID should be returned with a camelCase name
			if val, ok := data[0]["keyid"]; ok {
				data[0]["keyID"] = val
				delete(data[0], "keyid")
			}
			returnJSON(w, req, data[0])
		} else {
			http.Error(w, "Key not found", http.StatusNotFound)
		}
		return
	}
	// If no key id is given, return a list of all keys.
	data, err := QueryList(vars.db, "SELECT "+strings.Join(columns, ",")+
		" FROM apikeys WHERE ownerid=$1 ORDER BY created ASC", access.OwnerID())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fields["ipRanges"] {
		for i := range data {
			data[i]["ipRanges"], err = QueryColumn(vars.db, "SELECT iprange FROM apikey_ips WHERE keyid=$1",
				data[i]["keyid"])
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	for i := range data {
		if val, ok := data[i]["keyid"]; ok {
			// Remove the keyID column if they didn't ask for it
			if !fields["keyID"] {
				delete(data[i], "keyid")
			} else {
				// keyID should be returned with a camelCase name
				data[i]["keyID"] = val
				delete(data[i], "keyid")
			}
		}
	}
	returnJSON(w, req, data)
}

type apiKeyParams struct {
	comment, filter sql.NullString
	expires         pq.NullTime
	readonly        bool
	ipranges        []net.IPNet
}

func (vars *apiMethodKeys) create(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// Parse the parameters
	p := vars.parseParameters(w, req)
	if p == nil {
		return
	}
	var newKeyID int
	// Start a transaction
	err := utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {
		// Insert the new key
		err := tx.QueryRow("INSERT INTO apikeys(key,ownerid,readonly,comment,filter,expires) "+
			"VALUES($1,$2,$3,$4,$5,$6) RETURNING keyid",
			utility.RandomStringID(), access.OwnerID(), p.readonly,
			p.comment, p.filter, p.expires).Scan(&newKeyID)
		if err != nil {
			return err
		}
		// Insert the ip ranges
		for _, r := range p.ipranges {
			_, err = tx.Exec("INSERT INTO apikey_ips(keyid,iprange) VALUES($1,$2)",
				newKeyID, r.String())
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Location", req.URL.RequestURI()+"/"+strconv.Itoa(newKeyID))
	http.Error(w, "", http.StatusCreated) // 201 Created
}

func (vars *apiMethodKeys) update(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// parse the key id from the URL
	var keyID int
	var err error
	if i := strings.LastIndex(req.URL.Path, "/keys/"); i > -1 {
		keyID, err = strconv.Atoi(req.URL.Path[i+6:])
		if err != nil {
			http.Error(w, "Invalid Key ID: "+req.URL.Path[i+6:], http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "Missing key in URL path", http.StatusUnprocessableEntity)
		return
	}
	// parse the rest of the parameters
	p := vars.parseParameters(w, req)
	if p == nil {
		return
	}
	// Do you own the key?
	var ownerID, key sql.NullString
	err = vars.db.QueryRow("SELECT ownerid,key FROM apikeys WHERE keyid=$1", keyID).Scan(&ownerID, &key)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err != sql.ErrNoRows && ownerID.String != access.OwnerID() {
		http.Error(w, "This isn't your key.", http.StatusForbidden)
		return
	}
	// Start a transaction
	var rows int64
	err = utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {
		// Perform the update
		res, err := tx.Exec("UPDATE apikeys SET readonly=$1,comment=$2,filter=$3,expires=$4 "+
			"WHERE keyid=$5", p.readonly, p.comment, p.filter, p.expires, keyID)
		if err != nil {
			return err
		}
		rows, err = res.RowsAffected()
		if err != nil {
			return err
		}
		// Update the ip ranges
		_, err = tx.Exec("DELETE FROM apikey_ips WHERE keyid=$1", keyID)
		if err != nil {
			return err
		}
		for _, r := range p.ipranges {
			_, err = tx.Exec("INSERT INTO apikey_ips(keyid,iprange) VALUES($1,$2)", keyID, r.String())
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	invalidateCacheForKey(key.String)
	// Return
	if rows > 0 {
		http.Error(w, "", http.StatusNoContent)
	} else {
		http.Error(w, "Key not found", http.StatusNotFound)
	}
}

func (vars *apiMethodKeys) delete(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// parse the key id from the URL
	var keyID int
	var err error
	if i := strings.LastIndex(req.URL.Path, "/keys/"); i > -1 {
		keyID, err = strconv.Atoi(req.URL.Path[i+6:])
		if err != nil {
			http.Error(w, "Invalid Key ID: "+req.URL.Path[i+6:], http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "Missing key in URL path", http.StatusUnprocessableEntity)
		return
	}
	// Do you own the key?
	var ownerID, key sql.NullString
	err = vars.db.QueryRow("SELECT ownerid, key FROM apikeys WHERE keyid=$1", keyID).Scan(&ownerID, &key)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err != sql.ErrNoRows && ownerID.String != access.OwnerID() {
		http.Error(w, "This isn't your key.", http.StatusForbidden)
		return
	}
	// perform the query
	res, err := vars.db.Exec("DELETE FROM apikeys WHERE keyid=$1 AND ownerid=$2",
		keyID, access.OwnerID())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	invalidateCacheForKey(key.String)
	// Return
	if rows > 0 {
		http.Error(w, "", http.StatusNoContent)
	} else {
		http.Error(w, "Key not found", http.StatusNotFound)
	}
}

func formValue(form url.Values, caseInsensitiveKey string) string {
	for k, v := range form {
		if strings.EqualFold(k, caseInsensitiveKey) {
			return v[0]
		}
	}
	return ""
}

func (vars *apiMethodKeys) parseParameters(w http.ResponseWriter, req *http.Request) *apiKeyParams {
	// Parse parameter values
	// If one or more parameters have invalid values, will send an http response
	// with a JSON object with error messages.
	var params apiKeyParams
	paramErrors := make(map[string]string, 0)
	err := req.ParseForm()
	if err != nil && req.ContentLength > 0 {
		http.Error(w, fmt.Sprintf("Unable to parse the form data: %s", err.Error()),
			http.StatusBadRequest)
		return nil
	}
	comment := formValue(req.PostForm, "comment")
	if comment != "" {
		params.comment.String = comment
		params.comment.Valid = true
	}
	filter := formValue(req.PostForm, "filter")
	if filter != "" {
		params.filter.String = filter
		params.filter.Valid = true
		_, _, err := buildSQLWhere(filter, nil)
		if err != nil {
			paramErrors["filter"] = err.message
		}
	}
	expires := formValue(req.PostForm, "expires")
	if expires != "" {
		// First, try the full RFC3339 format
		tm, err := time.Parse(time.RFC3339, expires)
		if err != nil {
			// Plan B: try just YYYY-MM-DD
			tm, err = time.Parse("2006-01-02", expires)
			if err != nil {
				paramErrors["expires"] = "Unable to parse the time as RFC3339 or yyyy-mm-dd"
			}
		}
		if !tm.IsZero() {
			params.expires.Time = tm
			params.expires.Valid = true
		}
	}
	ipranges := formValue(req.PostForm, "ipRanges")
	if ipranges != "" {
		// split the iprange list on any combination of commas and all types of whitespace
		ar := regexp.MustCompile("[\\s\\,]+").Split(ipranges, -1)
		params.ipranges = make([]net.IPNet, 0, len(ar))
		for _, s := range ar {
			if len(s) == 0 {
				continue
			}
			// try to parse each entry
			ip, ipnet, err := net.ParseCIDR(s)
			if err != nil {
				paramErrors["ipRanges"] = "\"" + s + "\" is incorrect, should be CIDR format"
				continue
			}
			if !bytes.Equal(ip.To16(), ipnet.IP.To16()) {
				paramErrors["ipRanges"] = fmt.Sprintf("The ip address can't have bits set "+
					"to the right side of the netmask. Try %v\"}", ipnet.IP)
			}
			params.ipranges = append(params.ipranges, *ipnet)
		}
	}
	r := formValue(req.PostForm, "readonly")
	params.readonly = r == "" || isTrueish(r)
	if len(paramErrors) > 0 {
		returnJSON(w, req, paramErrors, http.StatusBadRequest)
		return nil
	}
	return &params
}
