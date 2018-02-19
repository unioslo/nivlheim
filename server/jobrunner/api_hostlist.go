package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type apiMethodHostList struct {
	db *sql.DB
}

var apiHostListSourceFields = []string{"ipAddress", "hostname", "lastseen", "os", "osEdition",
	"kernel", "vendor", "model", "serialNo", "certfp",
	"clientVersion"}

func (vars *apiMethodHostList) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	/*
		fields, hErr := unpackFieldParam(req.FormValue("fields"),
			apiHostListSourceFields)
		if hErr != nil {
			http.Error(w, hErr.message, hErr.code)
			return
		}
	*/
	err := req.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	where, qparams := buildSQLWhere(&apiHostListSourceFields, &req.Form)

	fmt.Fprintln(w, where)
	for i, v := range qparams {
		fmt.Fprintf(w, "$%d = %s\n", i+1, v)
	}
}

// Build the WHERE part of the SQL statement based on parameters.
// Must support "*" as a wildcard, and if a value starts with "!"
// it should be interpreted as NOT.
func buildSQLWhere(fields *[]string, form *url.Values) (string, []interface{}) {
	dbFieldNames := map[string]string{
		"ipAddress":     "ipaddr",
		"osEdition":     "os_edition",
		"serialNo":      "serialno",
		"clientVersion": "client_version",
	}
	where := make([]string, 0)
	qparams := make([]interface{}, 0)
	// Here, the "fields" map is also used as a list of fields that
	// the user can supply filter values for.
	for _, key := range *fields {
		for _, value := range (*form)[key] {
			not := ' '
			if len(value) > 0 && value[0] == '!' {
				not = '!'
				value = value[1:]
			}
			name, ok := dbFieldNames[key]
			if !ok {
				name = key
			}
			if strings.Index(value, "*") > -1 {
				// The value has wildcards
				parts := make([]string, 0)
				for _, valuePart := range strings.Split(value, "*") {
					if len(valuePart) > 0 {
						qparams = append(qparams, valuePart)
						parts = append(parts, fmt.Sprintf("$%d", len(qparams)))
					}
				}
				joined := strings.Join(parts, "||'%'||")
				if strings.HasPrefix(value, "*") {
					joined = "'%'||" + joined
				}
				if strings.HasSuffix(value, "*") {
					joined += "||'%'"
				}
				if not == '!' {
					where = append(where, fmt.Sprintf("%s NOT LIKE %s",
						name, joined))
				} else {
					where = append(where, fmt.Sprintf("%s LIKE %s",
						name, joined))
				}
			} else {
				qparams = append(qparams, value)
				where = append(where, fmt.Sprintf("%s%c= $%d", name, not,
					len(qparams)))
			}
		}
	}
	return strings.Join(where, " AND "), qparams
}
