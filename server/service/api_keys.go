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
		vars.read(w, req, access)
	case httpPOST:
		vars.create(w, req, access)
	case httpPUT:
		vars.update(w, req, access)
	case httpDELETE:
		vars.delete(w, req, access)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (vars *apiMethodKeys) read(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// See which fields I'm supposed to return
	fields, hErr := unpackFieldParam(req.FormValue("fields"), []string{
		"keyID", "key", "comment", "readonly", "expires", "ipRanges",
		"groups", "ownerGroup"})
	if hErr != nil {
		http.Error(w, hErr.message, hErr.code)
		return
	}

	// The function scanOneRow assumes the fields are ordered as in this statement:
	const selectStatement = "SELECT keyid, key, ownergroup, comment, " +
		"readonly, expires, all_groups, groups, " +
		"array(SELECT iprange FROM apikey_ips WHERE keyid=k.keyid) as ipranges " +
		"FROM apikeys k "

	// Read a key with a specific id?
	if i := strings.LastIndex(req.URL.Path, "/keys/"); i > -1 {

		// Parse the key ID from the url
		keyID, err := strconv.Atoi(req.URL.Path[i+6:])
		if err != nil {
			http.Error(w, "Invalid Key ID: "+req.URL.Path[i+6:], http.StatusBadRequest)
			return
		}

		// Read the database table row
		row := vars.db.QueryRow(selectStatement+"WHERE keyid=$1", keyID)

		// Scan/parse the row
		result, ownergroup, err := scanOneRow(row, fields, vars.db)

		// Handle errors
		if err == sql.ErrNoRows {
			http.Error(w, "Key not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Verify access
		if !access.HasAccessToGroup(ownergroup) {
			http.Error(w, "You don't have access to this key.", http.StatusForbidden)
			return
		}

		// Return the result
		returnJSON(w, req, result)
		return
	}

	// If no key ID was given, select a list of all keys you have access to.
	var rows *sql.Rows
	var err error
	if access.HasAccessToAllGroups() {
		// Admins can see all keys
		rows, err = vars.db.Query(selectStatement)
	} else {
		rows, err = vars.db.Query(selectStatement +
			"WHERE ownergroup IN (" + access.GetGroupListForSQLWHERE() + ")")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Process the returned rows from the database
	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		// Scan/parse the current row and append it to the result array
		rowMap, _, err := scanOneRow(rows, fields, vars.db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		result = append(result, rowMap)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	returnJSON(w, req, result)
}

// RowScanner lets you pass either a *sql.Row or *sql.Rows type to a function
type RowScanner interface {
	Scan(dest ...interface{}) error
}

func scanOneRow(row RowScanner, fields map[string]bool, db *sql.DB) (map[string]interface{}, string, error) {
	// Scan the table row into local variables
	var keyID int
	var key, ownergroup, comment sql.NullString
	var readonly, allGroups sql.NullBool
	var expires pq.NullTime
	var groups, ipranges []string
	err := row.Scan(&keyID, &key, &ownergroup, &comment, &readonly, &expires,
		&allGroups, pq.Array(&groups), pq.Array(&ipranges))
	if err != nil {
		return nil, "", err
	}
	// Put together the response
	result := make(map[string]interface{}, len(fields))
	if fields["keyID"] {
		result["keyID"] = keyID
	}
	if fields["key"] {
		result["key"] = jsonString(key)
	}
	if fields["ownerGroup"] {
		result["ownerGroup"] = jsonString(ownergroup)
	}
	if fields["readonly"] {
		result["readonly"] = jsonBool(readonly)
	}
	if fields["expires"] {
		result["expires"] = jsonTime(expires)
	}
	if fields["comment"] {
		result["comment"] = jsonString(comment)
	}
	if fields["groups"] {
		if groups == nil {
			groups = make([]string, 0)
		}
		result["groups"] = groups
		result["allGroups"] = jsonBool(allGroups)
	}
	if fields["ipRanges"] {
		result["ipRanges"] = ipranges
	}
	return result, ownergroup.String, nil
}

type apiKeyParams struct {
	comment    sql.NullString
	expires    pq.NullTime
	readonly   bool
	ipranges   []net.IPNet
	ownerGroup sql.NullString
	groups     []string
	allGroups  bool
}

func (vars *apiMethodKeys) create(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	// Parse the parameters
	p := vars.parseParameters(w, req)
	if p == nil {
		return
	}

	// If ownerGroup isn't supplied, that's an error
	if !p.ownerGroup.Valid || strings.TrimSpace(p.ownerGroup.String) == "" {
		http.Error(w, "Missing required parameter: ownerGroup", http.StatusBadRequest)
		return
	}

	// ownerGroup must be one of the groups you have access to
	if !access.HasAccessToGroup(p.ownerGroup.String) {
		http.Error(w, "You can't create a key that belongs to a group you don't have access to: "+
			p.ownerGroup.String, http.StatusForbidden)
		return
	}

	// You must also have access to all the groups you attempt to give the key access to
	for _, g := range p.groups {
		if !access.HasAccessToGroup(g) {
			http.Error(w, "You can't create a key that has access to "+
				"a group you don't have access to: "+g, http.StatusForbidden)
			return
		}
	}

	// If you try to create a key with access to ALL groups, you must have access to all groups
	if p.allGroups && !access.HasAccessToAllGroups() {
		http.Error(w, "You don't have access to all groups, so you can't create a key that does.", http.StatusForbidden)
		return
	}

	// Start a transaction
	var newKeyID int
	err := utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {
		// Insert the new key
		err := tx.QueryRow("INSERT INTO apikeys(key,ownergroup,readonly,comment,expires,groups,all_groups) "+
			"VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING keyid",
			utility.RandomStringID(), p.ownerGroup, p.readonly,
			p.comment, p.expires, pq.Array(p.groups), p.allGroups).Scan(&newKeyID)
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
	// parse the key ID from the URL
	var keyID int
	var err error
	if i := strings.LastIndex(req.URL.Path, "/keys/"); i > -1 {
		keyID, err = strconv.Atoi(req.URL.Path[i+6:])
		if err != nil {
			http.Error(w, "Invalid Key ID: "+req.URL.Path[i+6:], http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "Missing key ID in URL path", http.StatusUnprocessableEntity)
		return
	}

	// parse the rest of the parameters
	p := vars.parseParameters(w, req)
	if p == nil {
		return
	}

	// Read a few things about the existing key
	var ownerGroup, key sql.NullString
	var allGroups sql.NullBool
	err = vars.db.QueryRow("SELECT ownergroup,all_groups,key FROM apikeys WHERE keyid=$1", keyID).
		Scan(&ownerGroup, &allGroups, &key)
	if err == sql.ErrNoRows {
		http.Error(w, "Key not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Do you have access to the key?
	if !access.HasAccessToGroup(ownerGroup.String) {
		http.Error(w, "You don't have access to this key.", http.StatusForbidden)
		return
	}

	// If a new ownerGroup is supplied, it must be one of the groups you have access to
	newOwnerGroup := ownerGroup.String // default to old value
	if p.ownerGroup.Valid && strings.TrimSpace(p.ownerGroup.String) != "" {
		if !access.HasAccessToGroup(p.ownerGroup.String) {
			http.Error(w, "You can't give away a key to a group you don't have access to: "+
				p.ownerGroup.String, http.StatusBadRequest)
			return
		}
		newOwnerGroup = p.ownerGroup.String
	}

	// If the key has the all_groups flag set from before,
	// you're not allowed to edit it unless you have access to all groups.
	if allGroups.Bool && !access.HasAccessToAllGroups() {
		http.Error(w, "You are not allowed to modify this key, since it gives access to all groups.",
			http.StatusForbidden)
		return
	}

	// Only people with access to all groups can modify a key so it has access to all groups
	if p.allGroups && !access.HasAccessToAllGroups() {
		http.Error(w, "You are not allowed to create keys with access to all groups.",
			http.StatusForbidden)
		return
	}

	// Start a transaction
	var rows int64
	err = utility.RunInTransaction(vars.db, func(tx *sql.Tx) error {
		// Perform the update
		res, err := tx.Exec("UPDATE apikeys SET readonly=$1,comment=$2,expires=$3,"+
			"groups=$4,ownergroup=$5,all_groups=$6 WHERE keyid=$7",
			p.readonly, p.comment, p.expires, pq.Array(p.groups), newOwnerGroup,
			p.allGroups, keyID)
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
		http.Error(w, "Missing key ID in URL path", http.StatusUnprocessableEntity)
		return
	}

	// Read a few things about the existing key
	var ownerGroup, key sql.NullString
	var allGroups sql.NullBool
	err = vars.db.QueryRow("SELECT ownergroup,all_groups,key FROM apikeys WHERE keyid=$1", keyID).
		Scan(&ownerGroup, &allGroups, &key)
	if err == sql.ErrNoRows {
		http.Error(w, "Key not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Do you have access to the key?
	if !access.HasAccessToGroup(ownerGroup.String) {
		http.Error(w, "You don't have access to this key.", http.StatusForbidden)
		return
	}

	// If the key has the all_groups flag set from before,
	// you're not allowed to delete it unless also have that access.
	// Reason: You wouldn't be able to re-create it.
	// This rule basically exists to prevent people shooting themselves in the foot.
	if allGroups.Bool && !access.HasAccessToAllGroups() {
		http.Error(w, "You aren't allowed to delete this key, since it gives access to all groups and you wouldn't be able to re-create it.",
			http.StatusForbidden)
		return
	}

	// perform the query
	res, err := vars.db.Exec("DELETE FROM apikeys WHERE keyid=$1", keyID)
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

func ifFormValue(form url.Values, caseInsensitiveKey string) (string, bool) {
	for k, v := range form {
		if strings.EqualFold(k, caseInsensitiveKey) {
			return v[0], true
		}
	}
	return "", false
}

func formValue(form url.Values, caseInsensitiveKey string) string {
	for k, v := range form {
		if strings.EqualFold(k, caseInsensitiveKey) {
			// If multiple values were given, return them comma-separated.
			return strings.Join(v, ",")
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
	ownerGroup := formValue(req.PostForm, "ownerGroup")
	if ownerGroup != "" {
		params.ownerGroup.String = ownerGroup
		params.ownerGroup.Valid = true
	}
	comment := formValue(req.PostForm, "comment")
	if comment != "" {
		params.comment.String = comment
		params.comment.Valid = true
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
	groups := formValue(req.PostForm, "groups")
	if groups != "" {
		params.groups = strings.Split(groups, ",")
	}
	ag := formValue(req.PostForm, "allGroups")
	params.allGroups = ag != "" && isTrueish(ag)
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
