package main

import (
	"net/http"
	"os"
	"testing"
)

func TestApiMethodFile(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}

	tests := []apiCall{
		// Test that methods other than GET aren't allowed
		{
			methodAndPath: "POST /api/v2/file",
			body:          "content=yee-haw",
			expectStatus:  http.StatusMethodNotAllowed,
		},
		{
			methodAndPath: "PUT /api/v2/file",
			body:          "content=yee-haw",
			expectStatus:  http.StatusMethodNotAllowed,
		},
		// Try to retrieve a file, using the wrong case for the field name
		{
			methodAndPath: "GET /api/v2/file?certfp=aaaa&filename=autoexec.bat&fields=cOnTeNt",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"content\":\"@echo off\"}",
		},
		// Retrieve multiple files
		{
			methodAndPath: "GET /api/v2/file?filename=autoexec.bat&fields=content",
			expectStatus:  http.StatusOK,
			expectJSON:    `[{"content":"@echo off"},{"content":"device=emm386.exe"}]`,
		},
		// Can't use raw format when getting multiple files
		{
			methodAndPath: "GET /api/v2/file?filename=autoexec.bat&format=raw",
			expectStatus:  400,
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()
	db.Exec("INSERT INTO files(certfp,filename,content) " +
		"VALUES('aaaa','autoexec.bat','@echo off')," +
		"('bbbb','autoexec.bat','device=emm386.exe')")

	mux := http.NewServeMux()
	mux.Handle("/api/v2/file", wrapRequireAuth(&apiMethodFile{db: db}, db))
	testAPIcalls(t, mux, tests)
}
