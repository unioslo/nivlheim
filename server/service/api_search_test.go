package main

import (
	"net/http"
	"os"
	"testing"
)

func TestSearchCaseSensitivity(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}

	db := getDBconnForTesting(t)
	defer db.Close()

	// Prepare some data for the tests
	var fileID int64 = 1
	var certfp = "ABFF"
	var content = "Sugar and Spice and Everything Nice"
	_, err := db.Exec("INSERT INTO files(fileid,certfp,content) VALUES($1,$2,$3)", fileID, certfp, content)
	if err != nil {
		t.Error(err)
		return
	}
	_, err = db.Exec("INSERT INTO hostinfo(certfp) VALUES($1)", certfp)
	if err != nil {
		t.Error(err)
		return
	}
	addFileToFastSearch(fileID, certfp, "/etc/whatever", content)
	fsReady = 1

	// Prepare a http muxer
	api := http.NewServeMux()
	api.Handle("/api/v2/search",
		wrapRequireAuth(&apiMethodSearch{db: db}, db))
	api.Handle("/api/v2/searchpage",
		wrapRequireAuth(&apiMethodSearchPage{db: db, devmode: false}, db))
	api.Handle("/api/v2/grep",
		wrapRequireAuth(&apiMethodGrep{db: db}, db))

	// Define some tests.
	// Test that the search methods match is case-insensitive, and that the returned content
	// retains the original mixed case.
	tests := []apiCall{
		{
			methodAndPath: "GET /api/v2/search?q=sPice&fields=content",
			expectStatus:  http.StatusOK,
			expectJSON:    "[{\"content\":\"" + content + "\"}]",
		},
		{
			methodAndPath: "GET /api/v2/searchpage?q=spiCe",
			expectStatus:  http.StatusOK,
			expectContent: "Spice",
		},
		{
			methodAndPath: "GET /api/v2/grep?q=spicE",
			expectStatus:  http.StatusOK,
			expectContent: content,
		},
	}

	// Run the tests
	testAPIcalls(t, api, tests)
}
