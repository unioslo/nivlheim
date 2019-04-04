package main

import (
	"net/http"
	"os"
	"testing"
)

func TestApiMethodIpRanges(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}
	tests := []apiCall{
		// Register a new ip range (error: Missing ipRange parameter)
		{
			methodAndPath: "POST /api/v2/settings/ipranges",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Register a new ip range (error: Range is in invalid format)
		{
			methodAndPath: "POST /api/v2/settings/ipranges",
			body:          "ipRange=192.168.0.0%2F255.255.0.0&comment=notright",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Register a new ip range (error: CIDR has bits to the right of mask)
		{
			methodAndPath: "POST /api/v2/settings/ipranges",
			body:          "ipRange=172.16.23.23%2F12&comment=notright",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Register a new ip range
		{
			methodAndPath: "POST /api/v2/settings/ipranges",
			body:          "ipRange=172.16.0.0%2F12&useDns=1&comment=blabla",
			expectStatus:  http.StatusCreated,
		},
		// Register a new ip range (error: CIDR is contained within existing)
		{
			methodAndPath: "POST /api/v2/settings/ipranges",
			body:          "ipRange=172.16.34.0%2F24",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Read the list
		{
			methodAndPath: "GET /api/v2/settings/ipranges?fields=ipRange,useDns,comment,ipRangeId",
			expectStatus:  http.StatusOK,
			expectJSON:    `{"ipRanges":[{"ipRange":"172.16.0.0/12","useDns":true,"comment":"blabla","ipRangeId":1}]}`,
		},
		// Verify error on missing fields parameter
		{
			methodAndPath: "GET /api/v2/settings/ipranges",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Update the ip range
		{
			methodAndPath: "PUT /api/v2/settings/ipranges/1",
			body:          "ipRange=192.168.0.0%2F16&useDns=0&comment=different",
			expectStatus:  http.StatusNoContent,
		},
		// Read it back
		{
			methodAndPath: "GET /api/v2/settings/ipranges?fields=ipRange,useDns,comment,ipRangeId",
			expectStatus:  http.StatusOK,
			expectJSON:    `{"ipRanges":[{"ipRange":"192.168.0.0/16","useDns":false,"comment":"different","ipRangeId":1}]}`,
		},
		// Delete (error: missing id in url)
		{
			methodAndPath: "DELETE /api/v2/settings/ipranges",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Delete it
		{
			methodAndPath: "DELETE /api/v2/settings/ipranges/1",
			expectStatus:  http.StatusNoContent,
		},
		// Read back the list, it should be empty
		{
			methodAndPath: "GET /api/v2/settings/ipranges?fields=ipRangeId",
			expectStatus:  http.StatusOK,
			expectJSON:    `{"ipRanges":[]}`,
		},
		// Delete nonexistent, should return 404
		{
			methodAndPath: "DELETE /api/v2/settings/ipranges/1",
			expectStatus:  http.StatusNotFound,
		},
		// Update nonexistent, should return 404
		{
			methodAndPath: "PUT /api/v2/settings/ipranges/1",
			body:          "ipRange=192.168.0.0%2F16&useDns=0&comment=different",
			expectStatus:  http.StatusNotFound,
		},
		// Register a new ip range (with a twist: The parameter names are terribly mixed case)
		{
			methodAndPath: "POST /api/v2/settings/ipranges",
			body:          "IpRaNge=55.55.55.55%2F32&cOmmEnt=fiftyfive",
			expectStatus:  http.StatusCreated,
		},
		// Same as above, but update
		{
			methodAndPath: "PUT /api/v2/settings/ipranges/2",
			body:          "IpRaNge=66.66.66.66%2F32&cOmmEnt=sixtysix",
			expectStatus:  http.StatusNoContent,
		},
	}
	db := getDBconnForTesting(t)
	defer db.Close()
	mux := http.NewServeMux()
	mux.Handle("/", wrapRequireAdmin(&apiMethodIpRanges{db: db}, db))
	testAPIcalls(t, mux, tests)
}
