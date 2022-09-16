package main

import (
	"database/sql"
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
			methodAndPath: "GET /api/v2/hostlist?fields=hostname,duck",
			expectStatus:  http.StatusOK,
			expectJSON: "[{\"hostname\":\"bar.baz.no\",\"duck\":\"gladstone\"}," +
				"{\"hostname\":\"foo.bar.no\",\"duck\":\"donald\"}]",
		},
		// filter on a custom field
		{
			methodAndPath: "GET /api/v2/hostlist?fields=hostname,duck&duck=donald",
			expectStatus:  http.StatusOK,
			expectJSON:    "[{\"hostname\":\"foo.bar.no\", \"duck\": \"donald\"}]",
		},
		// filter on a custom field (that isn't in the list of returned fields)
		{
			methodAndPath: "GET /api/v2/hostlist?fields=hostname&duck=donald",
			expectStatus:  http.StatusOK,
			expectJSON:    "[{\"hostname\":\"foo.bar.no\"}]",
		},
		// Group query
		{
			methodAndPath: "GET /api/v2/hostlist?fields=hostname&count=1&sort=hostname",
			expectStatus:  http.StatusOK,
			expectJSON: "[{\"hostname\":\"bar.baz.no\",\"count\":1}," +
				"{\"hostname\":\"foo.bar.no\",\"count\":1}]",
		},
		// Group query, reverse sorted
		{
			methodAndPath: "GET /api/v2/hostlist?fields=hostname&count=1&sort=-hostname",
			expectStatus:  http.StatusOK,
			expectJSON: "[{\"hostname\":\"foo.bar.no\",\"count\":1}," +
				"{\"hostname\":\"bar.baz.no\",\"count\":1}]",
		},
		// Group query on a custom field
		{
			methodAndPath: "GET /api/v2/hostlist?fields=town&count=1",
			expectStatus:  http.StatusOK,
			expectJSON:    `[{"town":"duckville","count":2}]`,
		},
		// Group query on two fields
		{
			methodAndPath: "GET /api/v2/hostlist?fields=osEdition,town&count=1",
			expectStatus:  http.StatusOK,
			expectJSON:    `[{"town":"duckville","osEdition":"workstation","count":2}]`,
		},
		// Group on a field where the column name differs from the API name
		{
			methodAndPath: "GET /api/v2/hostlist?fields=osEdition&count=1",
			expectStatus:  http.StatusOK,
			expectJSON:    `[{"osEdition":"workstation","count":2}]`,
		},
		// Test with an access profile that should prevent some hosts from being counted
		{
			methodAndPath:  "GET /api/v2/hostlist?fields=osEdition&count=1",
			expectStatus:   http.StatusOK,
			expectJSON:     `[{"osEdition":"workstation","count":1}]`,
			sessionProfile: &AccessProfile{isAdmin: false, groups: map[string]bool{"foogroup": true}},
		},
		// Test with an access profile that should prevent some hosts from being counted
		{
			methodAndPath:  "GET /api/v2/hostlist?fields=osEdition&hostname=*baz*&count=1",
			expectStatus:   http.StatusOK,
			expectJSON:     "[]",
			sessionProfile: &AccessProfile{isAdmin: false, groups: map[string]bool{"foogroup": true}},
		},
		// Test POST
		{
			methodAndPath: "POST /api/v2/hostlist",
			body: `[{"createIfNotExists":true,"hostname":"postpostpost",` +
				`"os":"ExampleOS","ownerGroup":"mygroup"}]`,
			expectStatus:   http.StatusOK,
			expectContent:  "Updated 0 hosts, created 1 new hosts",
			sessionProfile: &AccessProfile{isAdmin: false, groups: map[string]bool{"mygroup": true}},
		},
		// Create a host without supplying ownerGroup - should fail
		{
			methodAndPath: "POST /api/v2/hostlist",
			body:          `[{"createIfNotExists":true,"hostname":"whatever"}]`,
			expectStatus:  http.StatusOK,
			expectContent: "Updated 0 hosts, created 0 new hosts, 1 errors.",
		},
		// Try to update a host I don't have access to, should fail
		{
			methodAndPath:  "POST /api/v2/hostlist",
			body:           `[{"hostname":"postpostpost","product":"laptop"}]`,
			sessionProfile: &AccessProfile{isAdmin: false, groups: map[string]bool{"foogroup": true}},
			expectStatus:   http.StatusOK,
			expectContent:  "Updated 0 hosts, created 0 new hosts, 1 errors.",
		},
		// Try to update ownerGroup to another group I don't have access to, should fail
		{
			methodAndPath:  "POST /api/v2/hostlist",
			body:           `[{"hostname":"postpostpost","ownerGroup":"someoneElsesGroup"}]`,
			sessionProfile: &AccessProfile{isAdmin: false, groups: map[string]bool{"mygroup": true}},
			expectStatus:   http.StatusOK,
			expectContent:  "Updated 0 hosts, created 0 new hosts, 1 errors.",
		},
		// Try to update a custom field on a non-existent host
		{
			methodAndPath:  "POST /api/v2/hostlist",
			body:           `[{"hostname":"giraffe","duck":"Louie"}]`,
			sessionProfile: &AccessProfile{isAdmin: false, groups: map[string]bool{"mygroup": true}},
			expectStatus:   http.StatusOK,
			expectContent:  "Updated 0 hosts, created 0 new hosts, 0 errors.",
		},
		// Regression tests for a bug (See GitHub issue #151)
		{
			methodAndPath: "GET /api/v2/hostlist?fields=hostname,os,duck&ipAddress=129.240.98.*",
			expectStatus:  http.StatusOK,
		},
		{
			methodAndPath: "GET /api/v2/hostlist?fields=hostname,os&ipAddress=129.240.98.*",
			expectStatus:  http.StatusOK,
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()
	_, err := db.Exec("INSERT INTO hostinfo(certfp,hostname,os_edition,ownergroup) VALUES" +
		"('1111','foo.bar.no','workstation','foogroup')," +
		"('2222','bar.baz.no','workstation','bargroup')")
	if err != nil {
		t.Error(err)
	}
	_, err = db.Exec("INSERT INTO customfields(name) VALUES('duck'),('town')")
	if err != nil {
		t.Error(err)
	}
	_, err = db.Exec("INSERT INTO hostinfo_customfields(certfp,fieldid,value) VALUES" +
		"('1111',1,'donald'),('2222',1,'gladstone')," +
		"('1111',2,'duckville'),('2222',2,'duckville')")
	if err != nil {
		t.Error(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v2/hostlist", wrapRequireAuth(&apiMethodHostList{db: db, devmode: true}, db))
	testAPIcalls(t, mux, tests)

	var certfp sql.NullString
	err = db.QueryRow("SELECT certfp FROM hostinfo WHERE hostname='postpostpost'").Scan(&certfp)
	if err == sql.ErrNoRows {
		t.Error("POST hostlist failed, didn't find host in database")
	} else if err != nil {
		t.Error(err)
	} else if len(certfp.String) < 32 || len(certfp.String) > 40 {
		t.Errorf("Expected a certfp between 32 and 40 chars, got \"%s\"", certfp.String)
	}

}

func TestHideUnknownHosts(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}

	db := getDBconnForTesting(t)
	defer db.Close()
	_, err := db.Exec("INSERT INTO hostinfo(certfp,hostname,ipaddr,os_edition) VALUES" +
		"('1111','foo.bar.no','1.1.1.1','workstation')," +
		"('2222',null,'2.2.2.2','workstation')")
	if err != nil {
		t.Error(err)
	}

	testsWhenOptionOff := []apiCall{
		{
			methodAndPath: "GET /api/v2/hostlist?fields=hostname",
			expectStatus:  http.StatusOK,
			expectJSON:    `[{"hostname":"2.2.2.2"},{"hostname":"foo.bar.no"}]`,
		},
	}
	testsWhenOptionOn := []apiCall{
		{
			methodAndPath: "GET /api/v2/hostlist?fields=hostname",
			expectStatus:  http.StatusOK,
			expectJSON:    `[{"hostname":"foo.bar.no"}]`,
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v2/hostlist", wrapRequireAuth(&apiMethodHostList{db: db, devmode: true}, db))

	config.HideUnknownHosts = true
	testAPIcalls(t, mux, testsWhenOptionOn)
	config.HideUnknownHosts = false
	testAPIcalls(t, mux, testsWhenOptionOff)
}
