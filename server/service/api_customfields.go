package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

//  GET  /api/v0/customfields            - list all
//  POST /api/v0/customfields            - create a new
//  GET  /api/v0/customfields/<name>     - show details for one
//  PUT  /api/v0/customfields/<name>     - update(replace) one
//  DELETE  /api/v0/customfields/<name>  - delete one

type apiMethodCustomFieldsCollection struct {
	db *sql.DB
}

type apiMethodCustomFieldsItem struct {
	db *sql.DB
}

func (vars *apiMethodCustomFieldsCollection) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	switch req.Method {
	case httpGET:
		// List all
		fields, hErr := unpackFieldParam(req.FormValue("fields"), []string{"name", "filename", "regexp"})
		if hErr != nil {
			http.Error(w, hErr.message, hErr.code)
			return
		}
		keys := make([]string, len(fields))
		i := 0
		for k := range fields {
			if k == "filename" {
				k = "replace(filename,'%','*') as filename"
			}
			keys[i] = k
			i++
		}
		data, err := QueryList(vars.db, "SELECT "+strings.Join(keys, ",")+
			" FROM customfields ORDER BY name")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		returnJSON(w, req, data)

	case httpPOST:
		if !access.IsAdmin() {
			http.Error(w, "This operation is only allowed for admins.", http.StatusForbidden)
			return
		}

		// parse the POST parameters
		err := req.ParseForm()
		if err != nil && req.ContentLength > 0 {
			http.Error(w, fmt.Sprintf("Unable to parse the form data: %s", err.Error()),
				http.StatusBadRequest)
			return
		}
		// Create a new item. Check parameters
		requiredParams := []string{"name", "filename", "regexp"}
		missingParams := make([]string, 0)
		for _, paramName := range requiredParams {
			if formValue(req.PostForm, paramName) == "" {
				missingParams = append(missingParams, paramName)
			}
		}
		if len(missingParams) > 0 {
			http.Error(w, "Missing parameters: "+strings.Join(missingParams, ","), http.StatusBadRequest)
			return
		}
		// if the name contains special characters, it isn't valid
		name := formValue(req.PostForm, "name")
		if regexp.MustCompile("[^a-z0-9_]").MatchString(name) {
			http.Error(w, "name contains invalid characters", http.StatusBadRequest)
			return
		}
		// Everything checks out, insert
		filename := strings.Replace(formValue(req.PostForm, "filename"), "*", "%", -1)
		_, err = vars.db.Exec("INSERT INTO customfields(name, filename, regexp) VALUES($1,$2,$3)",
			name, filename, formValue(req.PostForm, "regexp"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Mark relevant files for re-parsing
		vars.db.Exec("UPDATE files SET parsed=false WHERE current AND filename LIKE $1", filename)
		// Return
		w.Header().Set("Location", req.URL.RequestURI()+"/"+name)
		http.Error(w, "", http.StatusCreated) // 201 Created

	default:
		http.Error(w, "Method not supported.", http.StatusBadRequest)
		return
	}
}

func (vars *apiMethodCustomFieldsItem) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	switch req.Method {
	case httpGET:
		// Return details for one item
		match := regexp.MustCompile("/(\\w+)$").FindStringSubmatch(req.URL.Path)
		if match == nil {
			http.Error(w, "Missing field name in URL path", http.StatusUnprocessableEntity)
			return
		}
		fields, hErr := unpackFieldParam(req.FormValue("fields"), []string{"name", "filename", "regexp"})
		if hErr != nil {
			http.Error(w, hErr.message, hErr.code)
			return
		}
		var name, filename, re sql.NullString
		err := vars.db.QueryRow("SELECT name, filename, regexp FROM customfields "+
			"WHERE name=$1", match[1]).Scan(&name, &filename, &re)
		switch {
		case err == sql.ErrNoRows:
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		result := make(map[string]interface{})
		if fields["name"] {
			result["name"] = jsonString(name)
		}
		if fields["filename"] {
			if filename.Valid {
				filename.String = strings.Replace(filename.String, "%", "*", -1)
			}
			result["filename"] = jsonString(filename)
		}
		if fields["regexp"] {
			result["regexp"] = jsonString(re)
		}
		returnJSON(w, req, result)

	case httpDELETE:
		if !access.IsAdmin() {
			http.Error(w, "This operation is only allowed for admins.", http.StatusForbidden)
			return
		}

		// Delete one item
		match := regexp.MustCompile("/(\\w+)$").FindStringSubmatch(req.URL.Path)
		if match == nil {
			http.Error(w, "Missing field name in URL path", http.StatusUnprocessableEntity)
			return
		}
		res, err := vars.db.Exec("DELETE FROM customfields WHERE name=$1", match[1])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rowsAffected == 0 {
			http.Error(w, "Not Found", http.StatusNotFound) // 404 Not Found
			return
		}
		http.Error(w, "OK", http.StatusNoContent) // 204 No Content

	case httpPUT:
		if !access.IsAdmin() {
			http.Error(w, "This operation is only allowed for admins.", http.StatusForbidden)
			return
		}

		// parse the PUT parameters
		err := req.ParseForm()
		if err != nil && req.ContentLength > 0 {
			http.Error(w, fmt.Sprintf("Unable to parse the form data: %s", err.Error()),
				http.StatusBadRequest)
			return
		}
		// Replace one item
		match := regexp.MustCompile("/(\\w+)$").FindStringSubmatch(req.URL.Path)
		if match == nil {
			http.Error(w, "Missing field name in URL path", http.StatusUnprocessableEntity)
			return
		}
		name := match[1]
		requiredParams := []string{"filename", "regexp"}
		missingParams := make([]string, 0)
		for _, paramName := range requiredParams {
			if formValue(req.PostForm, paramName) == "" {
				missingParams = append(missingParams, paramName)
			}
		}
		if len(missingParams) > 0 {
			http.Error(w, "Missing parameters: "+strings.Join(missingParams, ","), http.StatusBadRequest)
			return
		}
		newName := formValue(req.PostForm, "name")
		if newName == "" {
			newName = name
		}
		filename := strings.Replace(formValue(req.PostForm, "filename"), "*", "%", -1)
		res, err := vars.db.Exec("UPDATE customfields SET name=$1, filename=$2, regexp=$3 "+
			"WHERE name=$4", newName, filename, formValue(req.PostForm, "regexp"), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rowsAffected == 0 {
			http.Error(w, "Not Found", http.StatusNotFound) // 404 Not Found
			return
		}
		// Mark relevant files for re-parsing
		vars.db.Exec("UPDATE files SET parsed=false WHERE current AND filename LIKE $1", filename)
		http.Error(w, "OK", http.StatusNoContent) // 204 No Content

	default:
		http.Error(w, "Method not supported.", http.StatusBadRequest)
		return
	}
}
