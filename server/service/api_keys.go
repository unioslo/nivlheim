package main

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/lib/pq"
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
		"key", "comment", "filter", "readonly", "expires", "ipranges"})
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}
	if fields["ipranges"] {
		// TODO implement
		delete(fields, "ipranges")
	}
	columns := make([]string, len(fields))
	i := 0
	for k := range fields {
		columns[i] = k
		i++
	}
	// read key with a specific id?
	if i := strings.LastIndex(req.URL.Path, "/keys/"); i > -1 {
		key := req.URL.Path[i+6:]
		data, err := QueryList(vars.db, "SELECT "+strings.Join(columns, ",")+
			" FROM apikeys WHERE ownerid=$1 AND key=$2", access.OwnerID(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else if len(data) > 0 {
			returnJSON(w, req, data[0])
		} else {
			http.Error(w, "Key not found", http.StatusNotFound)
		}
		return
	}
	// if not, return a list
	data, err := QueryList(vars.db, "SELECT "+strings.Join(columns, ",")+
		" FROM apikeys WHERE ownerid=$1 ORDER BY created ASC", access.OwnerID())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	returnJSON(w, req, data)
}

func (vars *apiMethodKeys) create(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// Create a new item. There are no required parameters
	newkey := randomStringID()
	var comment, filter sql.NullString
	var expires pq.NullTime
	if req.FormValue("comment") != "" {
		comment.String = req.FormValue("comment")
		comment.Valid = true
	}
	if req.FormValue("filter") != "" {
		filter.String = req.FormValue("filter")
		filter.Valid = true
	}
	if req.FormValue("expires") != "" {
		tm, err := time.Parse(time.RFC3339, req.FormValue("expires"))
		if err != nil {
			http.Error(w, "expires: "+err.Error(), http.StatusBadRequest)
			return
		}
		expires.Time = tm
		expires.Valid = true
	}
	if req.FormValue("ipranges") != "" {
		//TODO implement
	}
	r := req.FormValue("readonly")
	readonly := r == "" || isTrueish(r)
	_, err := vars.db.Exec("INSERT INTO apikeys(key,ownerid,readonly,comment,filter,expires) "+
		"VALUES($1,$2,$3,$4,$5,$6)",
		newkey, access.OwnerID(), readonly, comment, filter, expires)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Return
	w.Header().Set("Location", req.URL.RequestURI()+"/"+newkey)
	http.Error(w, "", http.StatusCreated) // 201 Created
}

func (vars *apiMethodKeys) update(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// parse the key id
	var key string
	if i := strings.LastIndex(req.URL.Path, "/keys/"); i > -1 {
		key = req.URL.Path[i+6:]
	} else {
		http.Error(w, "Missing key in URL path", http.StatusUnprocessableEntity)
		return
	}
	// parse the rest of the parameters
	var comment, filter sql.NullString
	var expires pq.NullTime
	if req.FormValue("comment") != "" {
		comment.String = req.FormValue("comment")
		comment.Valid = true
	}
	if req.FormValue("filter") != "" {
		filter.String = req.FormValue("filter")
		filter.Valid = true
	}
	if req.FormValue("expires") != "" {
		tm, err := time.Parse(time.RFC3339, req.FormValue("expires"))
		if err != nil {
			http.Error(w, "expires: "+err.Error(), http.StatusBadRequest)
			return
		}
		expires.Time = tm
		expires.Valid = true
	}
	if req.FormValue("ipranges") != "" {
		//TODO implement
	}
	r := req.FormValue("readonly")
	readonly := r == "" || isTrueish(r)
	// Perform the update
	res, err := vars.db.Exec("UPDATE apikeys SET readonly=$3,comment=$4,filter=$5,expires=$6 "+
		"WHERE key=$1 AND ownerid=$2",
		key, access.OwnerID(), readonly, comment, filter, expires)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Return
	if rows > 0 {
		http.Error(w, "", http.StatusNoContent)
	} else {
		http.Error(w, "Key not found", http.StatusNotFound)
	}
}

func (vars *apiMethodKeys) delete(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// parse the key id
	var key string
	if i := strings.LastIndex(req.URL.Path, "/keys/"); i > -1 {
		key = req.URL.Path[i+6:]
	} else {
		http.Error(w, "Missing key in URL path", http.StatusUnprocessableEntity)
		return
	}
	// perform the query
	res, err := vars.db.Exec("DELETE FROM apikeys WHERE key=$1 AND ownerid=$2", key, access.OwnerID())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Return
	if rows > 0 {
		http.Error(w, "", http.StatusNoContent)
	} else {
		http.Error(w, "Key not found", http.StatusNotFound)
	}
}
