package main

import (
	"database/sql"
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

func (vars *apiMethodCustomFieldsCollection) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case httpGET:
		// List all
		data, err := QueryColumn(vars.db, "SELECT name FROM customfields ORDER BY name")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		returnJSON(w, req, data)

	case httpPOST:
		// Create a new item. Check parameters
		requiredParams := []string{"name", "filename", "regexp"}
		missingParams := make([]string, 0)
		for _, paramName := range requiredParams {
			if req.FormValue(paramName) == "" {
				missingParams = append(missingParams, paramName)
			}
		}
		if len(missingParams) > 0 {
			http.Error(w, "Missing parameters: "+strings.Join(missingParams, ","), http.StatusBadRequest)
			return
		}
		// if the name contains special characters, it isn't valid
		name := req.FormValue("name")
		if regexp.MustCompile("[^a-z0-9_]").MatchString(name) {
			http.Error(w, "name contains invalid characters", http.StatusBadRequest)
			return
		}
		// Everything checks out, insert
		_, err := vars.db.Exec("INSERT INTO customfields(name, filename, regexp) VALUES($1,$2,$3)",
			name, req.FormValue("filename"), req.FormValue("regexp"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Mark relevant files for re-parsing
		vars.db.Exec("UPDATE files SET parsed=false WHERE filename=$1",
			req.FormValue("filename"))
		// Return
		w.Header().Set("Location", req.URL.RequestURI()+"/"+name)
		http.Error(w, "", http.StatusCreated) // 201 Created

	default:
		http.Error(w, "Method not supported.", http.StatusBadRequest)
		return
	}
}

func (vars *apiMethodCustomFieldsItem) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case httpGET:
		// Return details for one item
		match := regexp.MustCompile("/(\\w+)$").FindStringSubmatch(req.URL.Path)
		if match == nil {
			http.Error(w, "Missing field name in URL path", http.StatusUnprocessableEntity)
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
		result["name"] = jsonString(name)
		result["filename"] = jsonString(filename)
		result["regexp"] = jsonString(re)
		returnJSON(w, req, result)

	case httpDELETE:
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
			if req.FormValue(paramName) == "" {
				missingParams = append(missingParams, paramName)
			}
		}
		if len(missingParams) > 0 {
			http.Error(w, "Missing parameters: "+strings.Join(missingParams, ","), http.StatusBadRequest)
			return
		}
		newName := req.FormValue("name")
		if newName == "" {
			newName = name
		}
		res, err := vars.db.Exec("UPDATE customfields SET name=$1, filename=$2, regexp=$3 "+
			"WHERE name=$4", newName, req.FormValue("filename"), req.FormValue("regexp"), name)
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
		vars.db.Exec("UPDATE files SET parsed=false WHERE filename=$1",
			req.FormValue("filename"))
		http.Error(w, "OK", http.StatusNoContent) // 204 No Content

	default:
		http.Error(w, "Method not supported.", http.StatusBadRequest)
		return
	}
}
