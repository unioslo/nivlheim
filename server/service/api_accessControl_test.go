package main

import (
	"net"
	"os"
	"testing"
	"time"
)

func TestApiAccessControl(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}

	adminAP := &AccessProfile{isAdmin: true}
	userAP := &AccessProfile{isAdmin: false, certs: map[string]bool{"1234": true}}
	expiredAP := &AccessProfile{isAdmin: false, certs: map[string]bool{"1234": true},
		expires: time.Now().Add(-time.Duration(1) * time.Minute)}
	readonlyAP := &AccessProfile{readonly: true, isAdmin: false, certs: map[string]bool{"1234": true}}
	restrictedIPAP := &AccessProfile{isAdmin: false, certs: map[string]bool{"1234": true},
		ipranges: []net.IPNet{{IP: []byte{192, 168, 0, 1}, Mask: []byte{255, 255, 255, 0}}}}

	tests := []apiCall{
		//============= test that expiration date/time is enforced ====
		{
			methodAndPath: "GET /api/v0/host?hostname=foo.acme.com&fields=hostname",
			accessProfile: expiredAP,
			expectStatus:  403,
			expectContent: "This key has expired",
		},
		//============= test that readonly is enforced =======
		{
			// delete as user with access
			methodAndPath: "DELETE /api/v0/host?hostname=foo.acme.com",
			accessProfile: readonlyAP,
			expectStatus:  403,
			expectContent: "This key can only be used for GET requests",
		},
		//============= test that ip restrictions are enforced ===
		{
			methodAndPath: "GET /api/v0/host?hostname=foo.acme.com&fields=hostname",
			accessProfile: restrictedIPAP,
			expectStatus:  403,
			expectContent: "This key can only be used from certain ip addresses",
		},
		//========= hostlist api =========
		{
			// Unauthorized users should get an error
			methodAndPath: "GET /api/v0/hostlist?fields=hostname",
			expectStatus:  401,
			runAsNotAuth:  true,
		},
		{
			// Admin should see all hosts
			methodAndPath: "GET /api/v0/hostlist?fields=hostname&sort=hostname",
			expectStatus:  200,
			accessProfile: adminAP,
			expectJSON:    "[{\"hostname\":\"bar.acme.com\"},{\"hostname\":\"foo.acme.com\"}]",
		},
		{
			// The user should only see the hosts they have access to
			methodAndPath: "GET /api/v0/hostlist?fields=hostname",
			expectStatus:  200,
			accessProfile: userAP,
			expectJSON:    "[{\"hostname\":\"foo.acme.com\"}]",
		},
		//============= file api ==============
		{
			// User requests details for a file they have access to
			methodAndPath: "GET /api/v0/file?hostname=foo.acme.com&filename=roadrunner&fields=content",
			accessProfile: userAP,
			expectStatus:  200,
			expectJSON:    "{\"content\":\"beep,beep\"}",
		},
		{
			// User requests details for a file they DON'T have access to
			methodAndPath: "GET /api/v0/file?hostname=bar.acme.com&filename=coyote&fields=content",
			accessProfile: userAP,
			expectStatus:  403,
		},
		{
			// Admin requests details for a file
			methodAndPath: "GET /api/v0/file?hostname=bar.acme.com&filename=coyote&fields=content",
			accessProfile: adminAP,
			expectStatus:  200,
		},
		{
			// Unauthorized request for a file
			methodAndPath: "GET /api/v0/file?hostname=bar.acme.com&filename=coyote&fields=content",
			runAsNotAuth:  true,
			expectStatus:  401,
		},
		//============= search api ==============
		{
			// unauthorized
			methodAndPath: "GET /api/v0/searchpage?q=beep",
			runAsNotAuth:  true,
			expectStatus:  401,
		},
		{
			// admin should get 2 hits
			methodAndPath: "GET /api/v0/searchpage?q=ep",
			accessProfile: adminAP,
			expectStatus:  200,
			expectContent: "\"numberOfHits\": 2,",
		},
		{
			// the regular user should get only 1 hit
			methodAndPath: "GET /api/v0/searchpage?q=ep",
			accessProfile: userAP,
			expectStatus:  200,
			expectContent: "\"numberOfHits\": 1,",
		},
		//======== host api =========
		{
			// User requests details for a host they have access to
			methodAndPath: "GET /api/v0/host?hostname=foo.acme.com&fields=hostname",
			accessProfile: userAP,
			expectStatus:  200,
			expectJSON:    "{\"hostname\":\"foo.acme.com\"}",
		},
		{
			// User requests details for a host they DON'T have access to
			methodAndPath: "GET /api/v0/host?hostname=bar.acme.com&fields=hostname",
			accessProfile: userAP,
			expectStatus:  403,
		},
		{
			// Unauthorized
			methodAndPath: "GET /api/v0/host?hostname=foo.acme.com&fields=hostname",
			runAsNotAuth:  true,
			expectStatus:  401,
		},
		{
			// admin
			methodAndPath: "GET /api/v0/host?hostname=foo.acme.com&fields=hostname",
			accessProfile: adminAP,
			expectStatus:  200,
			expectJSON:    "{\"hostname\":\"foo.acme.com\"}",
		},
		{
			// delete as unauthorized
			methodAndPath: "DELETE /api/v0/host?hostname=bar.acme.com",
			runAsNotAuth:  true,
			expectStatus:  401,
		},
		{
			// delete as user without access
			methodAndPath: "DELETE /api/v0/host?hostname=bar.acme.com",
			accessProfile: userAP,
			expectStatus:  403,
		},
		{
			// delete as user with access
			methodAndPath: "DELETE /api/v0/host?hostname=foo.acme.com",
			accessProfile: userAP,
			expectStatus:  204,
		},
		{
			// delete as admin
			methodAndPath: "DELETE /api/v0/host?hostname=bar.acme.com",
			accessProfile: adminAP,
			expectStatus:  204,
		},
		//============= awaitingApproval api ============
		{
			methodAndPath: "GET /api/v0/awaitingApproval?fields=ipAddress",
			runAsNotAuth:  true,
			expectStatus:  401,
		},
		{
			methodAndPath: "GET /api/v0/awaitingApproval?fields=ipAddress",
			accessProfile: userAP,
			expectStatus:  403,
		},
		{
			methodAndPath: "GET /api/v0/awaitingApproval?fields=ipAddress",
			accessProfile: adminAP,
			expectStatus:  200,
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()
	_, err := db.Exec("INSERT INTO hostinfo(hostname,certfp) VALUES('foo.acme.com','1234'),('bar.acme.com','5678')")
	if err != nil {
		t.Error(err)
		return
	}
	_, err = db.Exec("INSERT INTO files(certfp,filename,content) VALUES('1234','roadrunner','beep,beep'),('5678','coyote','ep')")
	if err != nil {
		t.Error(err)
		return
	}
	loadContentForFastSearch(db)
	muxer := createAPImuxer(db, false)
	testAPIcalls(t, muxer, tests)
}
