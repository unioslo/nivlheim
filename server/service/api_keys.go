package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
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
		"key", "comment", "filter", "readonly", "expires", "ipranges"})
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}
	columns := make([]string, len(fields))
	i := 0
	for k := range fields {
		if k == "ipranges" {
			// ipranges isn't a column, but we'll need the "key" column in the SQL
			// to be able to select ipranges belonging to that key
			k = "key"
		}
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
			if fields["ipranges"] {
				data[0]["ipranges"], err = QueryColumn(vars.db, "SELECT iprange FROM apikey_ips WHERE key=$1", key)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				// Remove the key column if they didn't ask for it
				if !fields["key"] {
					delete(data[0], "key")
				}
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
	if fields["ipranges"] {
		for i := range data {
			data[i]["ipranges"], err = QueryColumn(vars.db, "SELECT iprange FROM apikey_ips WHERE key=$1",
				data[i]["key"])
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	// Remove the key column if they didn't ask for it
	if !fields["key"] {
		for i := range data {
			delete(data[i], "key")
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
	p, err := vars.parseParameters(w, req)
	if err != nil {
		return
	}
	// Create a new key ID
	newkey := randomStringID()
	// Start a transaction
	err = utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {
		// Insert the new key
		_, err = tx.Exec("INSERT INTO apikeys(key,ownerid,readonly,comment,filter,expires) "+
			"VALUES($1,$2,$3,$4,$5,$6)",
			newkey, access.OwnerID(), p.readonly, p.comment, p.filter, p.expires)
		if err != nil {
			return err
		}
		// Insert the ip ranges
		for _, r := range p.ipranges {
			_, err = tx.Exec("INSERT INTO apikey_ips(key,iprange) VALUES($1,$2)",
				newkey, r.String())
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
	p, err := vars.parseParameters(w, req)
	if err != nil {
		return
	}
	// Start a transaction
	var rows int64
	err = utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {
		// Perform the update
		res, err := tx.Exec("UPDATE apikeys SET readonly=$3,comment=$4,filter=$5,expires=$6 "+
			"WHERE key=$1 AND ownerid=$2",
			key, access.OwnerID(), p.readonly, p.comment, p.filter, p.expires)
		if err != nil {
			return err
		}
		rows, err = res.RowsAffected()
		if err != nil {
			return err
		}
		// Update the ip ranges
		_, err = tx.Exec("DELETE FROM apikey_ips WHERE key=$1", key)
		if err != nil {
			return err
		}
		for _, r := range p.ipranges {
			_, err = tx.Exec("INSERT INTO apikey_ips(key,iprange) VALUES($1,$2)", key, r.String())
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
	// Return
	if rows > 0 {
		http.Error(w, "", http.StatusNoContent)
	} else {
		http.Error(w, "Key not found", http.StatusNotFound)
	}
}

func (vars *apiMethodKeys) delete(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// parse the key id from the URL
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

func (vars *apiMethodKeys) parseParameters(w http.ResponseWriter, req *http.Request) (*apiKeyParams, error) {
	// Parse parameter values
	// If one or more parameters have invalid values, will send an http response
	// with a JSON object with error messages.
	var params apiKeyParams
	paramErrors := make(map[string]string, 0)
	if req.FormValue("comment") != "" {
		params.comment.String = req.FormValue("comment")
		params.comment.Valid = true
	}
	if req.FormValue("filter") != "" {
		params.filter.String = req.FormValue("filter")
		params.filter.Valid = true
	}
	if req.FormValue("expires") != "" {
		// First, try the full RFC3339 format
		tm, err := time.Parse(time.RFC3339, req.FormValue("expires"))
		if err != nil {
			// Plan B: try just YYYY-MM-DD
			tm, err = time.Parse("2006-01-02", req.FormValue("expires"))
			if err != nil {
				paramErrors["expires"] = "Unable to parse the time as either RFC3339 or yyyy-mm-dd"
			}
		}
		if !tm.IsZero() {
			params.expires.Time = tm
			params.expires.Valid = true
		}
	}
	if req.FormValue("ipranges") != "" {
		// split the iprange list on any combination of commas and all types of whitespace
		ar := regexp.MustCompile("[\\s\\,]+").Split(req.FormValue("ipranges"), -1)
		params.ipranges = make([]net.IPNet, len(ar))
		for i, s := range ar {
			// try to parse each entry
			ip, ipnet, err := net.ParseCIDR(s)
			if err != nil {
				paramErrors["ipranges"] = "\"" + s + "\" is incorrect, should be CIDR format"
				continue
			}
			if !bytes.Equal(ip.To16(), ipnet.IP.To16()) {
				paramErrors["ipranges"] = fmt.Sprintf("The ip address can't have bits set "+
					"to the right side of the netmask. Try %v\"}", ipnet.IP)
			}
			params.ipranges[i] = *ipnet
		}
	}
	r := req.FormValue("readonly")
	params.readonly = r == "" || isTrueish(r)
	if len(paramErrors) > 0 {
		returnJSON(w, req, paramErrors, http.StatusBadRequest)
		return nil, errors.New("")
	}
	return &params, nil
}
