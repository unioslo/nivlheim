package main

import (
	"net/http"
	"testing"
)

func TestApiMethodFile(t *testing.T) {
	tests := []apiCall{
		// Test that methods other than GET aren't allowed
		{
			methodAndPath: "POST /api/v0/file",
			body:          "content=yee-haw",
			expectStatus:  http.StatusMethodNotAllowed,
		},
		{
			methodAndPath: "PUT /api/v0/file",
			body:          "content=yee-haw",
			expectStatus:  http.StatusMethodNotAllowed,
		},
		// Try to retrieve a file, using the wrong case for the field name
		{
			methodAndPath: "GET /api/v0/file?certfp=aaaa&filename=autoexec.bat&fields=cOnTeNt",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"content\":\"@echo off\"}",
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()
	db.Exec("INSERT INTO files(certfp,filename,content) VALUES('aaaa','autoexec.bat','@echo off');")

	mux := http.NewServeMux()
	mux.Handle("/api/v0/file", wrapRequireAuth(&apiMethodFile{db: db}, db))
	testAPIcalls(t, mux, tests)
}
