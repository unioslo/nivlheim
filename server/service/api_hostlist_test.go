package main

import (
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
			query:  "os=Fedora&vendor!=Dell*",
			sql:    "os = $1 AND vendor NOT LIKE $2||'%'",
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
	}

	for _, w := range tests {
		result, params, err := buildSQLWhere(w.query)
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
