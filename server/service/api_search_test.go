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
	const fileID int64 = 1
	const certfp = "ABFF"
	const certfp2 = "FFAB"
	const filename = "/etc/whatever"
	const content = "Sugar and Spice and Everything Nice"
	const content2 = "Night and Fog and definitely no sugar"
	const hostname = "acme.example.com"
	const hostname2 = "blammo.example.com"
	_, err := db.Exec("INSERT INTO files(fileid,filename,certfp,content) "+
		"VALUES($1,$2,$3,$4)", fileID, filename, certfp, content)
	if err != nil {
		t.Error(err)
		return
	}
	_, err = db.Exec("INSERT INTO hostinfo(certfp,hostname) VALUES($1,$2)",
		certfp, hostname)
	if err != nil {
		t.Error(err)
		return
	}
	addFileToFastSearch(fileID, certfp, filename, content)

	// The same file with different content on another host
	_, err = db.Exec("INSERT INTO files(fileid,filename,certfp,content) "+
		"VALUES($1,$2,$3,$4)", fileID+1, filename, certfp2, content2)
	if err != nil {
		t.Error(err)
		return
	}
	_, err = db.Exec("INSERT INTO hostinfo(certfp,hostname) VALUES($1,$2)",
		certfp2, hostname2)
	if err != nil {
		t.Error(err)
		return
	}
	addFileToFastSearch(fileID+1, certfp2, filename, content2)

	// Done with creating data
	fsReady = 1

	// Prepare a http muxer
	api := createAPImuxer(db, false)

	// Define some tests.
	tests := []apiCall{
		// Test that the search methods match is case-insensitive, and that the returned content
		// retains the original mixed case.
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
		{
			methodAndPath: "GET /api/v2/msearch?q1=sPice&fields=certfp",
			expectStatus:  http.StatusOK,
			expectJSON:    `[{"certfp":"ABFF"}]`,
		},
		// Test the multi-stage search
		// First: AND (intersection)
		{
			methodAndPath: "GET /api/v2/msearch?q1=sugar&f1=" + filename +
				"&op2=and&q2=spice&f2=" + filename +
				"&fields=hostname",
			expectStatus: http.StatusOK,
			expectJSON:   `[{"hostname":"` + hostname + `"}]`,
		},
		// OR (union)
		{
			methodAndPath: "GET /api/v2/msearch?q1=sugar&f1=" + filename +
				"&op2=or&q2=nice&f2=" + filename +
				"&fields=hostname",
			expectStatus: http.StatusOK,
			expectJSON:   `[{"hostname":"` + hostname + `"},{"hostname":"` + hostname2 + `"}]`,
		},
		// SUB (difference)
		{
			methodAndPath: "GET /api/v2/msearch?q1=sugar&f1=" + filename +
				"&op2=sub&q2=nice&f2=" + filename +
				"&fields=hostname",
			expectStatus: http.StatusOK,
			expectJSON:   `[{"hostname":"` + hostname2 + `"}]`,
		},
	}

	// Run the tests
	testAPIcalls(t, api, tests)
}
