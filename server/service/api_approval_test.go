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
			methodAndPath: "GET /api/v2/manualApproval?fields=ipAddress,approvalId",
			expectStatus:  200,
			expectJSON:    "{\"manualApproval\":[{\"approvalId\":1,\"ipAddress\":\"4.3.2.1\"},{\"approvalId\":2,\"ipAddress\":\"8.7.6.5\"}]}",
		},
		{
			// approve the first machine
			methodAndPath: "PATCH /api/v2/manualApproval/1",
			body:          "hostname=baz.example.com&approved=true",
			expectStatus:  204,
		},
		{
			// PUT shouldn't work
			methodAndPath: "PUT /api/v2/manualApproval/1",
			expectStatus:  405, // method not allowed
		},
		{
			// read the list, and filter on approved=null. should have 1 entry now.
			methodAndPath: "GET /api/v2/manualApproval?fields=approvalId,approved&approved=null",
			expectStatus:  200,
			expectJSON:    "{\"manualApproval\":[{\"approvalId\":2,\"approved\":null}]}",
		},
		{
			// deny the other machine
			methodAndPath: "PATCH /api/v2/manualApproval/2",
			body:          "approved=false",
			expectStatus:  204,
		},
		{
			// read the list. should be empty now.
			methodAndPath: "GET /api/v2/manualApproval?fields=hostname&approved=null",
			expectStatus:  200,
			expectJSON:    "{\"manualApproval\":[]}",
		},
		{
			// pre-approve a new machine
			methodAndPath: "POST /api/v2/manualApproval",
			body:          "hostname=acme.example.com&ipAddress=11.22.33.44&approved=true",
			expectStatus:  204,
		},
		{
			// read the full list
			methodAndPath: "GET /api/v2/manualApproval?fields=hostname,approved",
			expectStatus:  200,
			expectJSON:    "{\"manualApproval\":[{\"hostname\":\"baz.example.com\",\"approved\":true},{\"hostname\":\"bar.example.com\",\"approved\":false},{\"hostname\":\"acme.example.com\",\"approved\":true}]}",
		},
		{
			// read the list, filter on approved=null. should be empty since the last machine was already pre-approved
			methodAndPath: "GET /api/v2/manualApproval?fields=hostname,ipAddress,approvalId&approved=null",
			expectStatus:  200,
			expectJSON:    "{\"manualApproval\":[]}",
		},
		{
			// remove an entry
			methodAndPath: "DELETE /api/v2/manualApproval/1",
			expectStatus:  204,
		},
		{
			// verify that the deleted item is no longer on the list
			methodAndPath: "GET /api/v2/manualApproval?fields=hostname,approved",
			expectStatus:  200,
			expectJSON:    "{\"manualApproval\":[{\"hostname\":\"bar.example.com\",\"approved\":false},{\"hostname\":\"acme.example.com\",\"approved\":true}]}",
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()

	db.Exec("INSERT INTO waiting_for_approval(ipaddr,hostname,received)" +
		" VALUES('4.3.2.1','foo.example.com',now()),('8.7.6.5','bar.example.com',now())")

	mux := http.NewServeMux()
	mux.Handle("/api/v2/manualApproval", &apiMethodApproval{db: db})
	mux.Handle("/api/v2/manualApproval/", &apiMethodApproval{db: db})
	testAPIcalls(t, mux, tests)
}
