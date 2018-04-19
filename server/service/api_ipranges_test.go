package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestApiMethodIpRanges(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}
	type apiCall struct {
		methodAndPath, body string
		expectStatus        int
		expectJSON          string
	}
	tests := []apiCall{
		// Register a new ip range (error: Missing ipRange parameter)
		{
			methodAndPath: "POST /api/v0/settings/ipranges",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Register a new ip range (error: Range is in invalid format)
		{
			methodAndPath: "POST /api/v0/settings/ipranges",
			body:          "ipRange=192.168.0.0%2F255.255.0.0&comment=notright",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Register a new ip range (error: CIDR has bits to the right of mask)
		{
			methodAndPath: "POST /api/v0/settings/ipranges",
			body:          "ipRange=172.16.23.23%2F12&comment=notright",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Register a new ip range
		{
			methodAndPath: "POST /api/v0/settings/ipranges",
			body:          "ipRange=172.16.0.0%2F12&useDns=1&comment=blabla",
			expectStatus:  http.StatusCreated,
		},
		// Register a new ip range (error: CIDR is contained within existing)
		{
			methodAndPath: "POST /api/v0/settings/ipranges",
			body:          "ipRange=172.16.34.0%2F24",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Read the list
		{
			methodAndPath: "GET /api/v0/settings/ipranges?fields=ipRange,useDns,comment,ipRangeId",
			expectStatus:  http.StatusOK,
			expectJSON:    `{"ipRanges":[{"ipRange":"172.16.0.0/12","useDns":true,"comment":"blabla","ipRangeId":1}]}`,
		},
		// Verify error on missing fields parameter
		{
			methodAndPath: "GET /api/v0/settings/ipranges",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Update the ip range
		{
			methodAndPath: "PUT /api/v0/settings/ipranges/1",
			body:          "ipRange=192.168.0.0%2F16&useDns=0&comment=different",
			expectStatus:  http.StatusNoContent,
		},
		// Read it back
		{
			methodAndPath: "GET /api/v0/settings/ipranges?fields=ipRange,useDns,comment,ipRangeId",
			expectStatus:  http.StatusOK,
			expectJSON:    `{"ipRanges":[{"ipRange":"192.168.0.0/16","useDns":false,"comment":"different","ipRangeId":1}]}`,
		},
		// Delete (error: missing id in url)
		{
			methodAndPath: "DELETE /api/v0/settings/ipranges",
			expectStatus:  http.StatusUnprocessableEntity,
		},
		// Delete it
		{
			methodAndPath: "DELETE /api/v0/settings/ipranges/1",
			expectStatus:  http.StatusNoContent,
		},
		// Read back the list, it should be empty
		{
			methodAndPath: "GET /api/v0/settings/ipranges?fields=ipRangeId",
			expectStatus:  http.StatusOK,
			expectJSON:    `{"ipRanges":[]}`,
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()
	api := apiMethodIpRanges{db: db}

	for _, tt := range tests {
		ar := strings.Split(tt.methodAndPath, " ")
		method, path := ar[0], ar[1]
		var rdr io.Reader
		if tt.body != "" {
			rdr = strings.NewReader(tt.body)
		}
		req, err := http.NewRequest(method, path, rdr)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		api.ServeHTTP(rr, req)
		if status := rr.Code; status != tt.expectStatus {
			t.Errorf("%s returned status %v, expected %v.\n%s",
				tt.methodAndPath, status, tt.expectStatus,
				rr.Body.String())
			continue
		}
		if tt.expectJSON != "" {
			isEqual, err := IsEqualJSON(rr.Body.String(), tt.expectJSON)
			if err != nil {
				t.Error(err)
			}
			if !isEqual {
				t.Errorf("Got result %s,\nexpected %s", rr.Body.String(),
					tt.expectJSON)
			}
		}
	}
}
