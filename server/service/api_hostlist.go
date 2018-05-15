package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

type apiMethodHostList struct {
	db      *sql.DB
	devmode bool
}

var hostInfoDbFieldNames = map[string]string{
	"ipAddress":     "ipaddr",
	"osEdition":     "os_edition",
	"serialNo":      "serialno",
	"clientVersion": "clientversion",
}

var apiHostListSourceFields = []string{"ipAddress", "hostname", "lastseen", "os", "osEdition",
	"kernel", "vendor", "model", "serialNo", "certfp",
	"clientVersion"}

func (vars *apiMethodHostList) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Grouping changes the whole SQL statement and returned fields
	if req.FormValue("group") != "" {
		performGroupQuery(w, req, vars.db, vars.devmode)
		return
	}

	fields, hErr := unpackFieldParam(req.FormValue("fields"),
		apiHostListSourceFields)
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	err := req.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	where, qparams, hErr := buildSQLWhere(req.URL.RawQuery)
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	statement := "SELECT ipaddr, hostname, lastseen, os, os_edition, " +
		"kernel, vendor, model, serialno, certfp, clientversion " +
		"FROM hostinfo WHERE hostname IS NOT NULL"
	if len(where) > 0 {
		statement += " AND " + where
	}

	if sort := req.FormValue("sort"); sort != "" {
		var desc string
		if sort[0] == '-' {
			sort = sort[1:]
			desc = "DESC"
		}
		if sort[0] == '+' {
			sort = sort[1:]
			// order is ASC by default
		}
		if contains(sort, apiHostListSourceFields) {
			h, ok := hostInfoDbFieldNames[sort]
			if ok {
				sort = h
			}
		} else {
			http.Error(w, "Unsupported sort field", http.StatusUnprocessableEntity)
			return
		}
		statement += fmt.Sprintf(" ORDER BY %s %s", sort, desc)
	} else {
		// Default to sorting by hostname, ascending
		statement += fmt.Sprintf(" ORDER BY hostname")
	}

	if req.FormValue("limit") != "" {
		var limit int
		if limit, err = strconv.Atoi(req.FormValue("limit")); err == nil {
			statement += fmt.Sprintf(" LIMIT %d", limit)
		} else {
			http.Error(w, "Invalid limit value", http.StatusBadRequest)
			return
		}
	}
	if req.FormValue("offset") != "" {
		var offset int
		if offset, err = strconv.Atoi(req.FormValue("offset")); err == nil {
			statement += fmt.Sprintf(" OFFSET %d", offset)
		} else {
			http.Error(w, "Invalid offset value", http.StatusBadRequest)
			return
		}
	}

	if vars.devmode {
		//log.Println(statement)
		//log.Print(qparams)
	}

	rows, err := vars.db.Query(statement, qparams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	result := make([]map[string]interface{}, 0)
	for rows.Next() {
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
		result = append(result, res)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	returnJSON(w, req, result)
}

func performGroupQuery(w http.ResponseWriter, req *http.Request,
	db *sql.DB, devmode bool) {
	if req.FormValue("fields") != "" {
		http.Error(w, "Can't combine group and fields parameters",
			http.StatusUnprocessableEntity)
		return
	}

	err := req.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	where, qparams, hErr := buildSQLWhere(req.URL.RawQuery)
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	group := req.FormValue("group")

	if !contains(group, apiHostListSourceFields) {
		http.Error(w, "Unsupported group field", http.StatusUnprocessableEntity)
		return
	}
	g, ok := hostInfoDbFieldNames[group]
	if ok {
		group = g
	}
	statement := "SELECT " + group + ", count(*) FROM hostinfo " +
		"WHERE hostname IS NOT NULL"
	if len(where) > 0 {
		statement += " AND " + where
	}
	statement += " GROUP BY " + group

	if devmode {
		log.Println(statement)
		log.Print(qparams)
	}

	rows, err := db.Query(statement, qparams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	result := make(map[string]int, 0)
	for rows.Next() {
		var groupName sql.NullString
		var count int
		err = rows.Scan(&groupName, &count)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if groupName.Valid {
			result[groupName.String] = count
		} else {
			result["null"] = count
		}
	}
	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	returnJSON(w, req, result)
}

// Build the WHERE part of the SQL statement based on parameters.
// - Supports "*" as a wildcard
// - If a value starts with "!" it means not equal to or not like
// - If a value starts with "<" or ">" it affects the comparison
func buildSQLWhere(queryString string) (string, []interface{}, *httpError) {
	// This slice will hold multiple clauses that will be ANDed together after
	where := make([]string, 0)
	// This slice will hold parameter values for the query
	qparams := make([]interface{}, 0)

	re := regexp.MustCompile("^(\\w+)([=!<>]{1,2})(.+)$")
	for _, pair := range strings.Split(queryString, "&") {
		un, err := url.QueryUnescape(pair)
		if err == nil {
			pair = un
		}
		m := re.FindStringSubmatch(pair)
		if m == nil || err != nil {
			return "", nil, &httpError{
				code:    http.StatusBadRequest,
				message: "Syntax error: " + pair,
			}
		}
		name := m[1]
		if name == "fields" || name == "sort" ||
			name == "limit" || name == "offset" || name == "group" {
			continue
		}
		operator := m[2]
		ok := false
		for _, s := range []string{"=", "!=", "<", ">"} {
			if s == operator {
				ok = true
				break
			}
		}
		if !ok {
			return "", nil, &httpError{
				code:    http.StatusBadRequest,
				message: "Unsupported operator: " + operator,
			}
		}
		validFieldName := false
		for _, key := range apiHostListSourceFields {
			if strings.EqualFold(key, name) {
				validFieldName = true
				break
			}
		}
		if !validFieldName {
			return "", nil, &httpError{
				message: "Unsupported field name: " + name,
				code:    http.StatusUnprocessableEntity,
			}
		}
		// the name of the field in the database
		colname, ok := hostInfoDbFieldNames[name]
		if !ok {
			colname = name
		}
		// Wildcards?
		value := m[3]
		if strings.Index(value, "*") > -1 {
			// The value contains wildcards
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
			if operator == "!=" {
				where = append(where, fmt.Sprintf("%s NOT LIKE %s",
					colname, joined))
			} else if operator == "=" {
				where = append(where, fmt.Sprintf("%s LIKE %s",
					colname, joined))
			} else {
				return "", nil, &httpError{
					message: "Can't use operator '" + operator + "' with wildcards ('*')",
					code:    http.StatusBadRequest,
				}
			}
		} else {
			// The value doesn't contain wildcards.
			if name == "lastseen" {
				// lastseen relative time magic. Examples:
				// >2h = more than 2 hours ago
				// <30m = less than 30 minutes ago
				// supported time units: s(seconds), m(minutes), h(hours), d(days)
				var count int
				var unit string
				_, err := fmt.Sscanf(value, "%d%s", &count, &unit)
				if err != nil || len(unit) > 1 {
					return "", nil, &httpError{
						message: "Wrong format for lastseen parameter",
						code:    http.StatusBadRequest,
					}
				}
				where = append(where,
					fmt.Sprintf("now()-interval '%d%s' %s lastseen",
						count, unit, operator))
			} else if value == "null" {
				if operator == "=" {
					where = append(where, colname+" IS NULL")
				} else if operator == "!=" {
					where = append(where, colname+" IS NOT NULL")
				} else {
					return "", nil, &httpError{
						message: "Unsupported operator for null value",
						code:    http.StatusBadRequest,
					}
				}
			} else {
				if strings.Index(value, ",") > -1 && operator == "=" {
					q := make([]string, 0)
					for _, s := range strings.Split(value, ",") {
						qparams = append(qparams, s)
						q = append(q, fmt.Sprintf("$%d", len(qparams)))
					}
					where = append(where, fmt.Sprintf("%s IN (%s)", colname,
						strings.Join(q, ",")))
				} else {
					qparams = append(qparams, value)
					where = append(where, fmt.Sprintf("%s %s $%d", colname,
						operator, len(qparams)))
				}
			}
		}
	}
	sql := strings.Join(where, " AND ")
	return sql, qparams, nil
}
