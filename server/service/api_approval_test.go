package main

import (
	"net/http"
	"os"
	"testing"
)

func TestApprovalAPI(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}

	tests := []apiCall{
		{
			// read the list. Should have 2 entries, inserted before the tests are run.
			methodAndPath: "GET /api/v0/awaitingApproval?fields=ipAddress,approvalId",
			expectStatus:  200,
			expectJSON:    "{\"awaitingApproval\":[{\"approvalId\":1,\"ipAddress\":\"4.3.2.1\"},{\"approvalId\":2,\"ipAddress\":\"8.7.6.5\"}]}",
		},
		{
			// approve the machine. That should make it disappear from the list.
			methodAndPath: "PUT /api/v0/awaitingApproval/1",
			body:          "hostname=baz.example.com",
			expectStatus:  204,
		},
		{
			// read the list. should have 1 entry now.
			methodAndPath: "GET /api/v0/awaitingApproval?fields=approvalId",
			expectStatus:  200,
			expectJSON:    "{\"awaitingApproval\":[{\"approvalId\":2}]}",
		},
		{
			// deny the other machine. That should make it disappear from the list.
			methodAndPath: "DELETE /api/v0/awaitingApproval/2",
			expectStatus:  204,
		},
		{
			// read the list. should be empty now.
			methodAndPath: "GET /api/v0/awaitingApproval?fields=hostname",
			expectStatus:  200,
			expectJSON:    "{\"awaitingApproval\":[]}",
		},
		{
			// pre-approve a new machine
			methodAndPath: "POST /api/v0/awaitingApproval",
			body:          "hostname=acme.example.com&ipAddress=11.22.33.44",
			expectStatus:  204,
		},
		{
			// read the list. should be empty since the machine was already pre-approved
			methodAndPath: "GET /api/v0/awaitingApproval?fields=hostname,ipAddress,approvalId",
			expectStatus:  200,
			expectJSON:    "{\"awaitingApproval\":[]}",
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()

	db.Exec("INSERT INTO waiting_for_approval(ipaddr,hostname,received)" +
		" VALUES('4.3.2.1','bar.example.com',now()),('8.7.6.5','baz.example.com',now())")

	mux := http.NewServeMux()
	mux.Handle("/api/v0/awaitingApproval", &apiMethodAwaitingApproval{db: db})
	mux.Handle("/api/v0/awaitingApproval/", &apiMethodAwaitingApproval{db: db})
	testAPIcalls(t, mux, tests)
}
