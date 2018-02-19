package main

import (
	"net/url"
	"reflect"
	"testing"
)

func TestBuildSQLWhere(t *testing.T) {
	values := make(url.Values)
	values.Set("hostname", "a*b*c")
	result, params := buildSQLWhere(&apiHostListSourceFields, &values)
	expected := "hostname LIKE $1||'%'||$2||'%'||$3"
	if expected != result {
		t.Fatalf("Wrong SQL. Got %s, expected %s", result, expected)
	}
	expectedParams := []interface{}{"a", "b", "c"}
	if !reflect.DeepEqual(expectedParams, params) {
		t.Fatalf("Wrong SQL params. Got %v, expected %v", params, expectedParams)
	}
}
