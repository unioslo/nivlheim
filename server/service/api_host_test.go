package main

import (
	"net/http"
	"os"
	"testing"
)

func TestHostAPI(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}

	tests := []apiCall{
		{
			methodAndPath: "GET /api/v2/host/foo.example.com?fields=hostname,overrideHostname",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"hostname\":\"foo.example.com\",\"overrideHostname\":null}",
		},
		{
			// Use PATCH to change the value of the overrideHostname field.
			// This call should fail because the name is already taken.
			methodAndPath: "PATCH /api/v2/host/foo.example.com",
			body:          "overrideHostname=bar.example.com",
			expectStatus:  http.StatusConflict,
		},
		{
			// Try again with a different name. This call should succeed.
			methodAndPath: "PATCH /api/v2/host/foo.example.com",
			body:          "overrideHostname=foo2.example.com",
			expectStatus:  http.StatusNoContent,
		},
		{
			// Read the object again, verify that the overrideHostname field got a value.
			methodAndPath: "GET /api/v2/host/foo.example.com?fields=hostname,overrideHostname",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"hostname\":\"foo.example.com\",\"overrideHostname\":\"foo2.example.com\"}",
		},
		{
			// Try to set the name to an empty string
			methodAndPath: "PATCH /api/v2/host/foo.example.com",
			body:          "overrideHostname=",
			expectStatus:  http.StatusNoContent,
		},
		{
			// Retrieve a host by fingerprint instead of hostname
			methodAndPath: "GET /api/v2/host/0123456789ABCDEF0123456789abcdef00001111?fields=hostname",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"hostname\":\"foo.example.com\"}",
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()
	_, err := db.Exec("INSERT INTO hostinfo(certfp,hostname) " +
		"VALUES('0123456789ABCDEF0123456789abcdef00001111','foo.example.com'), ('2222','bar.example.com')")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v2/host/", wrapRequireAuth(&apiMethodHost{db: db}, db))
	testAPIcalls(t, mux, tests)
}
