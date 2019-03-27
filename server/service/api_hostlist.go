package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type apiMethodHostList struct {
	db      *sql.DB
	devmode bool
}

type apiHostListStandardField struct {
	publicName string
	columnName string
}

var apiHostListStandardFields = []apiHostListStandardField{
	{publicName: "ipAddress", columnName: "ipaddr"},
	{publicName: "hostname", columnName: "hostname"},
	{publicName: "lastseen", columnName: "lastseen"},
	{publicName: "os", columnName: "os"},
	{publicName: "osEdition", columnName: "os_edition"},
	{publicName: "osFamily", columnName: "os_family"},
	{publicName: "kernel", columnName: "kernel"},
	{publicName: "manufacturer", columnName: "manufacturer"},
	{publicName: "product", columnName: "product"},
	{publicName: "serialNo", columnName: "serialno"},
	{publicName: "certfp", columnName: "certfp"},
	{publicName: "clientVersion", columnName: "clientversion"},
}

func (vars *apiMethodHostList) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	if req.Method != httpGET {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get a list of names and IDs of all defined custom fields
	customFields, customFieldIDs, err := getListOfCustomFields(vars.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Make a complete list of allowed field names (standard + custom)
	allowedFields := make([]string, len(apiHostListStandardFields))
	for i, f := range apiHostListStandardFields {
		allowedFields[i] = f.publicName
	}
	allowedFields = append(allowedFields, customFields...)

	// Grouping changes the whole SQL statement and what's returned,
	// so it is handled in a separate function
	if req.FormValue("group") != "" {
		performGroupQuery(w, req, vars.db, customFieldIDs, vars.devmode, access)
		return
	}

	// Parse the "fields" parameter and verify that all given field names are valid
	fields, hErr := unpackFieldParam(req.FormValue("fields"), allowedFields)
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	// Reduce the customfields array and map
	// down to the actual fields used in the query
	for i := 0; i < len(customFields); {
		name := customFields[i]
		if !strings.Contains(req.URL.RawQuery, name) {
			// This field wasn't in the query
			last := len(customFields) - 1
			customFields[i] = customFields[last]
			customFields = customFields[0:last]
			delete(customFieldIDs, name)
		} else {
			i++
		}
	}

	// allowedFields = all standard fields (regardless of what was specified)
	//                 + the custom fields that were asked for
	allowedFields = append(allowedFields[0:len(apiHostListStandardFields)], customFields...)

	//TODO: Why are we calling req.ParseForm?
	//      Let's see what happens if we don't!
	//err = req.ParseForm()
	//if err != nil {
	//	http.Error(w, err.Error(), http.StatusBadRequest)
	//	return
	//}

	// Call a function that assembles the "WHERE" clause with associated
	// parameter values based on the query
	where, qparams, hErr := buildSQLWhere(req.URL.RawQuery, allowedFields)
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	// Build the "SELECT ... " part of the statement, including custom fields
	// Start with the standard fields:
	temp := make([]string, 0, 10)
	for _, f := range apiHostListStandardFields {
		if f.columnName == "hostname" {
			temp = append(temp, "COALESCE(hostname,host(ipaddr)) as hostname")
		} else {
			temp = append(temp, f.columnName)
		}
	}
	statement := "SELECT " + strings.Join(temp, ",")

	// Then, append the custom fields
	for _, name := range customFields {
		statement = statement +
			", (SELECT value FROM hostinfo_customfields hc " +
			"WHERE hc.certfp=h.certfp AND hc.fieldid=" +
			strconv.Itoa(customFieldIDs[name]) + ") as " + name
	}
	statement += " FROM hostinfo h"

	if len(customFields) > 0 {
		// Must wrap the statement
		statement = "SELECT * FROM (" + statement + ") as foo "
	}

	// Add the WHERE clause, if any
	if len(where) > 0 {
		statement += " WHERE " + where
	}

	// Add an ORDER BY clause
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
		ok := false
		for _, f := range apiHostListStandardFields {
			if sort == f.publicName {
				sort = f.columnName
				ok = true
				break
			}
		}
		if !ok {
			_, ok = customFieldIDs[sort]
		}
		if !ok {
			http.Error(w, "Unsupported sort field", http.StatusUnprocessableEntity)
			return
		}
		statement += fmt.Sprintf(" ORDER BY %s %s", sort, desc)
	} else {
		// Default to sorting by hostname, ascending
		statement += fmt.Sprintf(" ORDER BY hostname")
	}

	/* LIMIT and OFFSET will work incorrectly if we have to filter the resultset
	   afterwards. (Because of access control.)
	   A workaround is to let Postgres return the entire dataset,
	   and implement limit/offset in the Go code after filtering.

		TODO: Perform tests to see if this makes the API function too slow,
			particularly with custom fields.
			A different approach could be to create a table in a temporary tablespace
			and fill it with the access list, and JOIN against this table.
			That way, LIMIT and OFFSET could be applied in the SQL statement.

	// Append LIMIT and OFFSET
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
	*/

	if vars.devmode {
		//	log.Println(statement)
		//	log.Print(qparams)
	}

	rows, err := vars.db.Query(statement, qparams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scanvars := make([]sql.NullString, len(cols))
	scanpointers := make([]interface{}, len(cols))
	for i := 0; i < len(scanvars); i++ {
		scanpointers[i] = &scanvars[i]
	}
	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		err = rows.Scan(scanpointers...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var hasAccessToThisRow = false
		res := make(map[string]interface{}, len(fields))
		var i = 0
		for _, f := range apiHostListStandardFields {
			if f.columnName == "certfp" {
				hasAccessToThisRow = access.HasAccessTo(scanvars[i].String)
			}
			if fields[f.publicName] {
				res[f.publicName] = jsonString(scanvars[i])
			}
			i++
		}
		if !hasAccessToThisRow {
			continue
		}
		for _, f := range customFields {
			if fields[f] {
				res[f] = jsonString(scanvars[i])
			}
			i++
		}
		result = append(result, res)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Limit, offset (see comment above)
	if req.FormValue("offset") != "" {
		var offset int
		if offset, err = strconv.Atoi(req.FormValue("offset")); err == nil {
			if offset < len(result) {
				result = result[offset:]
			} else {
				result = result[0:0]
			}
		} else {
			http.Error(w, "Invalid offset value", http.StatusBadRequest)
			return
		}
	}
	if req.FormValue("limit") != "" {
		var limit int
		if limit, err = strconv.Atoi(req.FormValue("limit")); err == nil {
			if limit > len(result) {
				limit = len(result)
			}
			result = result[0:limit]
		} else {
			http.Error(w, "Invalid limit value", http.StatusBadRequest)
			return
		}
	}

	returnJSON(w, req, result)
}

func performGroupQuery(w http.ResponseWriter, req *http.Request,
	db *sql.DB, customFieldIDs map[string]int, devmode bool, access *AccessProfile) {
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

	allowedFields := make([]string, len(apiHostListStandardFields))
	for i, f := range apiHostListStandardFields {
		allowedFields[i] = f.publicName
	}

	// Find the column name to group by
	group := req.FormValue("group")
	var colname string
	for _, f := range apiHostListStandardFields {
		if group == f.publicName {
			colname = f.columnName
		}
	}

	// Is it a custom field?
	var isCustomField = false
	if colname == "" {
		if _, ok := customFieldIDs[group]; !ok {
			http.Error(w, "Unsupported group field", http.StatusUnprocessableEntity)
			return
		}
		colname = group
		allowedFields = append(allowedFields, group)
		isCustomField = true
	}

	where, qparams, hErr := buildSQLWhere(req.URL.RawQuery, allowedFields)
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	var statement string
	if !isCustomField {
		statement = "SELECT " + colname + ", count(*) FROM hostinfo "
	} else {
		statement = "SELECT (SELECT value FROM hostinfo_customfields hc " +
			"WHERE hc.certfp=h.certfp AND hc.fieldid=" +
			strconv.Itoa(customFieldIDs[group]) + ") as " + colname +
			", count(*) FROM hostinfo h "
	}

	if len(where) > 0 {
		statement += " WHERE " + where
		if access != nil && !access.IsAdmin() {
			statement += " AND certfp IN (" + access.GetSQLWHERE() + ")"
		}
	} else if access != nil && !access.IsAdmin() {
		statement += " WHERE certfp IN (" + access.GetSQLWHERE() + ")"
	}
	statement += " GROUP BY " + colname

	if devmode {
		//log.Println(statement)
		//log.Print(qparams)
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
func buildSQLWhere(queryString string, allowedFields []string) (string, []interface{}, *httpError) {
	// This slice will hold multiple clauses that will be ANDed together after
	where := make([]string, 0)
	// This slice will hold parameter values for the query
	qparams := make([]interface{}, 0)

	re := regexp.MustCompile("^(\\w+)([=!<>]{1,2})(.+)$")
	for _, pair := range strings.Split(queryString, "&") {
		m := re.FindStringSubmatch(pair)
		if m == nil {
			return "", nil, &httpError{
				code:    http.StatusBadRequest,
				message: "Syntax error: " + pair,
			}
		}

		name, _ := url.QueryUnescape(m[1])
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

		if allowedFields != nil && !contains(name, allowedFields) {
			return "", nil, &httpError{
				message: "Unsupported field name: " + name,
				code:    http.StatusUnprocessableEntity,
			}
		}

		// The column name of the field may differ
		colname := name
		for _, f := range apiHostListStandardFields {
			if name == f.publicName {
				colname = f.columnName
			}
		}

		// Wildcards?
		value := m[3]
		if strings.Index(value, "*") > -1 {
			// The value contains wildcards
			value, _ = url.QueryUnescape(value)
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
				value, _ = url.QueryUnescape(value)
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
				// A bit of comma "magic":
				// Escaped commas are interpreted as part of the values,
				// and unescaped commas are interpreted as separators.
				if strings.Index(value, ",") > -1 && operator == "=" {
					q := make([]string, 0)
					for _, s := range strings.Split(value, ",") {
						s, _ = url.QueryUnescape(s)
						qparams = append(qparams, s)
						q = append(q, fmt.Sprintf("$%d", len(qparams)))
					}
					where = append(where, fmt.Sprintf("%s IN (%s)", colname,
						strings.Join(q, ",")))
				} else {
					value, _ = url.QueryUnescape(value)
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
