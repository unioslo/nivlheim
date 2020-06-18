package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
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
	expression string
}

var apiHostListStandardFields = []apiHostListStandardField{
	{publicName: "ipAddress", columnName: "ipaddr", expression: "host(ipaddr)"},
	{publicName: "hostname", columnName: "hostname", expression: "COALESCE(hostname,host(ipaddr))"},
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
	{publicName: "ownerGroup", columnName: "ownergroup"},
}

func (vars *apiMethodHostList) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	switch req.Method {
	case httpGET:
		vars.ServeGET(w, req, access)
	case httpPOST:
		vars.ServePOST(w, req, access)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (vars *apiMethodHostList) ServeGET(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
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

	// Call a function that assembles the "WHERE" clause with associated
	// parameter values based on the query
	where, qparams, hErr := buildSQLWhere(req.URL.RawQuery, allowedFields)
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	// Build the "SELECT ... " part of the statement, including custom fields
	// Start with the standard fields:
	temp := make([]string, 0, len(apiHostListStandardFields))
	for _, f := range apiHostListStandardFields {
		if f.expression != "" {
			temp = append(temp, f.expression + " AS " + f.columnName)
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
	var sortBy, desc string
	if sortBy = req.FormValue("sort"); sortBy != "" {
		if sortBy[0] == '-' {
			sortBy = sortBy[1:]
			desc = "DESC"
		}
		if sortBy[0] == '+' {
			sortBy = sortBy[1:]
			// order is ASC by default
		}
		ok := false
		for _, f := range apiHostListStandardFields {
			if sortBy == f.publicName {
				sortBy = f.columnName
				ok = true
				break
			}
		}
		if !ok {
			_, ok = customFieldIDs[sortBy]
		}
		if !ok {
			http.Error(w, "Unsupported sort field", http.StatusUnprocessableEntity)
			return
		}
		statement += fmt.Sprintf(" ORDER BY %s %s", sortBy, desc)
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

	rows, err := vars.db.Query(statement, qparams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		if vars.devmode {
			log.Println(statement)
			log.Print(qparams)
		}
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
			if f.columnName == "ownergroup" {
				hasAccessToThisRow = access.HasAccessToGroup(scanvars[i].String)
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

	if isTrueish(req.FormValue("count")) {
		// Count the number of unique occurrences of each result row,
		// pretty much like a SELECT DISTINCT ..., count(*) GROUP BY ... statement.
		// (Didn't use SQL because this way turned out to be easier.)
		// Step 1: Compute a hash value for each record
		resultHashes := make([]uint32, len(result))
		for i, m := range result {
			hash := fnv.New32a()
			// Exploit the fact that the marshaller returns fields in a deterministic order
			b, err := json.Marshal(m)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			hash.Write(b)
			resultHashes[i] = hash.Sum32()
		}
		// Step 2: Tally up the hash values
		countMap := make(map[uint32]int, 0)
		for _, hashValue := range resultHashes {
			countMap[hashValue]++
		}
		// Step 3: Make a new result array, add each unique record only once,
		//         and also add a "count" field.
		result2 := make([]map[string]interface{}, 0, len(countMap))
		for i := range result {
			hashValue := resultHashes[i]
			if countMap[hashValue] > 0 {
				m := result[i]
				m["count"] = countMap[hashValue]
				result2 = append(result2, m)
				countMap[hashValue] = 0
			}
		}
		// Step 4: Optionally sort the result
		if sortBy != "" { // The sort parameter was parsed earlier
			sort.Slice(result2, func(i, j int) bool {
				a, _ := result2[i][sortBy].(jsonString)
				b, _ := result2[j][sortBy].(jsonString)
				return (a.String < b.String) != (desc == "DESC")
			})
		}
		result = result2
	}

	returnJSON(w, req, result)
}

// Build the WHERE part of the SQL statement based on parameters.
// - Supports "*" as a wildcard
// - If a value starts with "!" it means not equal to or not like
// - If a value starts with "<" or ">" it affects the comparison
// - Can match one of several values if they are comma-separated
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
			name == "limit" || name == "offset" || name == "count" {
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
