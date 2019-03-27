package main

import (
	"net/http"
	"os"
	"reflect"
	"testing"
)

func TestBuildSQLWhere(t *testing.T) {
	type whereTest struct {
		query  string
		sql    string
		params []interface{}
		errmsg string
	}
	tests := []whereTest{
		whereTest{
			query: "hostname=a*b*c&hostname!=*dd*ee*",
			sql: "hostname LIKE $1||'%'||$2||'%'||$3 AND " +
				"hostname NOT LIKE '%'||$4||'%'||$5||'%'",
			params: []interface{}{"a", "b", "c", "dd", "ee"},
		},
		whereTest{
			query:  "os=Fedora&manufacturer!=Dell*",
			sql:    "os = $1 AND manufacturer NOT LIKE $2||'%'",
			params: []interface{}{"Fedora", "Dell"},
		},
		whereTest{
			query:  "fields=hostname,ipaddress&lastseen<2h",
			sql:    "now()-interval '2h' < lastseen",
			params: []interface{}{},
		},
		whereTest{
			query:  "lastseen<2mdb",
			sql:    "",
			params: nil,
			errmsg: "Wrong format for lastseen parameter",
		},
		whereTest{
			query:  "ipaddress$%#!",
			sql:    "",
			params: nil,
			errmsg: "Syntax error: ipaddress$%#!",
		},
		whereTest{
			query:  "nonexistentfield=123",
			sql:    "",
			params: nil,
			errmsg: "Unsupported field name: nonexistentfield",
		},
		whereTest{
			query:  "hostname>orange*",
			sql:    "",
			params: nil,
			errmsg: "Can't use operator '>' with wildcards ('*')",
		},
		whereTest{
			query:  "hostname=>foo",
			sql:    "",
			params: nil,
			errmsg: "Unsupported operator: =>",
		},
		whereTest{
			query:  "os=Debian+8,Debian+9",
			sql:    "os IN ($1,$2)",
			params: []interface{}{"Debian 8", "Debian 9"},
		},
		whereTest{
			query:  "osEdition=null",
			sql:    "os_edition IS NULL",
			params: []interface{}{},
		},
		whereTest{
			query:  "osEdition!=null",
			sql:    "os_edition IS NOT NULL",
			params: []interface{}{},
		},
		whereTest{
			// If commas are url encoded as %2C they should be considered part of the string
			query:  "manufacturer=VMWare%2C%20Inc.,Apple",
			sql:    "manufacturer IN ($1,$2)",
			params: []interface{}{"VMWare, Inc.", "Apple"},
		},
		whereTest{
			// If commas are url encoded as %2C they should be considered part of the string
			query:  "os=foo%2cbar%2Cbaz",
			sql:    "os = $1",
			params: []interface{}{"foo,bar,baz"},
		},
	}

	allowedFields := make([]string, len(apiHostListStandardFields))
	for i, f := range apiHostListStandardFields {
		allowedFields[i] = f.publicName
	}

	for _, w := range tests {
		result, params, err := buildSQLWhere(w.query, allowedFields)
		if err != nil && err.message != w.errmsg {
			if w.errmsg != "" {
				t.Errorf("Wrong error message.\n     Got: %s\n"+
					"Expected: %s", err.message, w.errmsg)
				continue
			} else {
				// Unexpected error
				t.Errorf("%s\n%s", err.message, w.query)
				continue
			}
		} else if w.errmsg != "" && err == nil {
			t.Errorf("Expected error \"%s\", got success.\n%s", w.errmsg, w.query)
			continue
		}
		if w.sql != result {
			t.Errorf("Wrong SQL.\n     Got: %s\n"+
				"Expected: %s", result, w.sql)
		}
		if !reflect.DeepEqual(w.params, params) {
			t.Errorf("Wrong SQL params. Got %v, expected %v", params, w.params)
		}
	}
}

func TestApiMethodHostList(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}

	tests := []apiCall{
		// a list that includes a custom field
		{
			methodAndPath: "GET /api/v0/hostlist?fields=hostname,duck",
			expectStatus:  http.StatusOK,
			expectJSON: "[{\"hostname\":\"bar.baz.no\",\"duck\":\"gladstone\"}," +
				"{\"hostname\":\"foo.bar.no\",\"duck\":\"donald\"}]",
		},
		// filter on a custom field
		{
			methodAndPath: "GET /api/v0/hostlist?fields=hostname,duck&duck=donald",
			expectStatus:  http.StatusOK,
			expectJSON:    "[{\"hostname\":\"foo.bar.no\", \"duck\": \"donald\"}]",
		},
		// filter on a custom field (that isn't in the list of returned fields)
		{
			methodAndPath: "GET /api/v0/hostlist?fields=hostname&duck=donald",
			expectStatus:  http.StatusOK,
			expectJSON:    "[{\"hostname\":\"foo.bar.no\"}]",
		},
		// Group query
		{
			methodAndPath: "GET /api/v0/hostlist?group=hostname",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"bar.baz.no\":1,\"foo.bar.no\":1}",
		},
		// Group query on a custom field
		{
			methodAndPath: "GET /api/v0/hostlist?group=town",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"duckville\":2}",
		},
		// Group on a field where the column name differs from the API name
		{
			methodAndPath: "GET /api/v0/hostlist?group=osEdition",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"workstation\":2}",
		},
		// Test with an access profile that should prevent some hosts from being counted
		{
			methodAndPath:  "GET /api/v0/hostlist?group=osEdition",
			expectStatus:   http.StatusOK,
			expectJSON:     "{\"workstation\":1}",
			sessionProfile: &AccessProfile{isAdmin: false, certs: map[string]bool{"1111": true}},
		},
		// Test with an access profile that should prevent some hosts from being counted
		{
			methodAndPath:  "GET /api/v0/hostlist?group=osEdition&hostname=*baz*",
			expectStatus:   http.StatusOK,
			expectJSON:     "{}",
			sessionProfile: &AccessProfile{isAdmin: false, certs: map[string]bool{"1111": true}},
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()
	_, err := db.Exec("INSERT INTO hostinfo(certfp,hostname,os_edition) " +
		"VALUES('1111','foo.bar.no','workstation'),('2222','bar.baz.no','workstation')")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO customfields(name) VALUES('duck'),('town')")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO hostinfo_customfields(certfp,fieldid,value) " +
		"VALUES('1111',1,'donald'),('2222',1,'gladstone')," +
		"('1111',2,'duckville'),('2222',2,'duckville')")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v0/hostlist", wrapRequireAuth(&apiMethodHostList{db: db, devmode: true}, db))
	testAPIcalls(t, mux, tests)
}
