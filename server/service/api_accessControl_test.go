package main

import (
	"os"
	"testing"
)

func TestApiAccessControl(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}

	adminAP := &AccessProfile{isAdmin: true}
	userAP := &AccessProfile{isAdmin: false, certs: map[string]bool{"1234": true}}

	tests := []apiCall{
		{
			methodAndPath: "GET /api/v0/hostlist?fields=hostname",
			expectStatus:  401,
			runAsNotAuth:  true,
		},
		{
			methodAndPath: "GET /api/v0/hostlist?fields=hostname&sort=hostname",
			expectStatus:  200,
			accessProfile: adminAP,
			expectJSON:    "[{\"hostname\":\"bar.acme.com\"},{\"hostname\":\"foo.acme.com\"}]",
		},
		{
			methodAndPath: "GET /api/v0/hostlist?fields=hostname",
			expectStatus:  200,
			accessProfile: userAP,
			expectJSON:    "[{\"hostname\":\"foo.acme.com\"}]",
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()
	_, err := db.Exec("INSERT INTO hostinfo(hostname,certfp) VALUES('foo.acme.com','1234'),('bar.acme.com','5678')")
	if err != nil {
		t.Error(err)
		return
	}

	muxer := createAPImuxer(db, false)
	testAPIcalls(t, muxer, tests)
}
