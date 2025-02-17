package main

import (
	"net"
	"net/http"
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
	userAP := &AccessProfile{isAdmin: false, groups: map[string]bool{"foogroup": true}}
	expiredAP := &AccessProfile{isAdmin: false, groups: map[string]bool{"bargroup": true},
		expires: time.Now().Add(-time.Duration(1) * time.Minute)}
	expiredAP.AllowAllIPs()
	readonlyAP := &AccessProfile{readonly: true, isAdmin: false, groups: map[string]bool{"foogroup": true}}
	readonlyAP.AllowAllIPs()
	restrictedIPAP := &AccessProfile{isAdmin: false, groups: map[string]bool{"foogroup": true},
		ipranges: []net.IPNet{{IP: []byte{192, 168, 0, 1}, Mask: []byte{255, 255, 255, 0}}}}

	tests := []apiCall{
		//============= test that expiration date/time is enforced ====
		{
			methodAndPath: "GET /api/v2/host/foo.acme.com?fields=hostname",
			accessProfile: expiredAP,
			expectStatus:  403,
			expectContent: "This key has expired",
		},
		//============= test that readonly is enforced =======
		{
			// delete as user with access
			methodAndPath: "DELETE /api/v2/host/foo.acme.com",
			accessProfile: readonlyAP,
			expectStatus:  403,
			expectContent: "This key can only be used for GET requests",
		},
		//============= test that ip restrictions are enforced ===
		{
			methodAndPath: "GET /api/v2/host/foo.acme.com?fields=hostname",
			accessProfile: restrictedIPAP,
			expectStatus:  403,
			expectContent: "This key can only be used from certain ip addresses",
		},
		{
			methodAndPath: "GET /api/v2/host/foo.acme.com?fields=hostname",
			remoteAddr:    "192.168.0.25",
			accessProfile: restrictedIPAP,
			expectStatus:  200,
			expectJSON:    "{\"hostname\":\"foo.acme.com\"}",
		},
		//========= hostlist api =========
		{
			// Unauthorized users should get an error
			methodAndPath: "GET /api/v2/hostlist?fields=hostname",
			expectStatus:  401,
			runAsNotAuth:  true,
		},
		{
			// Admin should see all hosts
			methodAndPath:  "GET /api/v2/hostlist?fields=hostname&sort=hostname",
			expectStatus:   200,
			sessionProfile: adminAP,
			expectJSON:     "[{\"hostname\":\"bar.acme.com\"},{\"hostname\":\"foo.acme.com\"}]",
		},
		{
			// The user should only see the hosts they have access to
			methodAndPath:  "GET /api/v2/hostlist?fields=hostname",
			expectStatus:   200,
			sessionProfile: userAP,
			expectJSON:     "[{\"hostname\":\"foo.acme.com\"}]",
		},
		//============= file api ==============
		{
			// User requests details for a file they have access to
			methodAndPath:  "GET /api/v2/file?hostname=foo.acme.com&filename=roadrunner&fields=content",
			sessionProfile: userAP,
			expectStatus:   200,
			expectJSON:     "{\"content\":\"beep,beep\"}",
		},
		{
			// User requests details for a file they DON'T have access to
			methodAndPath:  "GET /api/v2/file?hostname=bar.acme.com&filename=coyote&fields=content",
			sessionProfile: userAP,
			expectStatus:   403,
		},
		{
			// Admin requests details for a file
			methodAndPath:  "GET /api/v2/file?hostname=bar.acme.com&filename=coyote&fields=content",
			sessionProfile: adminAP,
			expectStatus:   200,
		},
		{
			// Unauthorized request for a file
			methodAndPath: "GET /api/v2/file?hostname=bar.acme.com&filename=coyote&fields=content",
			runAsNotAuth:  true,
			expectStatus:  401,
		},
		//============= search api ==============
		{
			// unauthorized
			methodAndPath: "GET /api/v2/searchpage?q=beep",
			runAsNotAuth:  true,
			expectStatus:  401,
		},
		{
			// admin should get 2 hits
			methodAndPath:  "GET /api/v2/searchpage?q=ep",
			sessionProfile: adminAP,
			expectStatus:   200,
			expectContent:  "\"numberOfHits\": 2,",
		},
		{
			// the regular user should get only 1 hit
			methodAndPath:  "GET /api/v2/searchpage?q=ep",
			sessionProfile: userAP,
			expectStatus:   200,
			expectContent:  "\"numberOfHits\": 1,",
		},
		//======== host api =========
		{
			// User requests details for a host they have access to
			methodAndPath:  "GET /api/v2/host/foo.acme.com?fields=hostname",
			sessionProfile: userAP,
			expectStatus:   200,
			expectJSON:     "{\"hostname\":\"foo.acme.com\"}",
		},
		{
			// User requests details for a host they DON'T have access to
			methodAndPath:  "GET /api/v2/host/bar.acme.com?fields=hostname",
			sessionProfile: userAP,
			expectStatus:   403,
		},
		{
			// Unauthorized
			methodAndPath: "GET /api/v2/host/foo.acme.com?fields=hostname",
			runAsNotAuth:  true,
			expectStatus:  401,
		},
		{
			// admin
			methodAndPath:  "GET /api/v2/host/foo.acme.com?fields=hostname",
			sessionProfile: adminAP,
			expectStatus:   200,
			expectJSON:     "{\"hostname\":\"foo.acme.com\"}",
		},
		{
			// delete as unauthorized
			methodAndPath: "DELETE /api/v2/host/bar.acme.com",
			runAsNotAuth:  true,
			expectStatus:  401,
		},
		{
			// delete as user without access
			methodAndPath:  "DELETE /api/v2/host/bar.acme.com",
			sessionProfile: userAP,
			expectStatus:   403,
		},
		{
			// delete as user with access
			methodAndPath:  "DELETE /api/v2/host/foo.acme.com",
			sessionProfile: userAP,
			expectStatus:   204,
		},
		{
			// delete as admin
			methodAndPath:  "DELETE /api/v2/host/bar.acme.com",
			sessionProfile: adminAP,
			expectStatus:   204,
		},
		//============= manualApproval api ============
		{
			methodAndPath: "GET /api/v2/manualApproval?fields=ipAddress",
			runAsNotAuth:  true,
			expectStatus:  401,
		},
		{
			methodAndPath:  "GET /api/v2/manualApproval?fields=ipAddress",
			sessionProfile: userAP,
			expectStatus:   403,
		},
		{
			methodAndPath:  "GET /api/v2/manualApproval?fields=ipAddress",
			sessionProfile: adminAP,
			expectStatus:   200,
		},
	}

	db := getDBconnForTesting(t)
	defer db.Close()
	_, err := db.Exec("INSERT INTO hostinfo(hostname,certfp,ownergroup) VALUES" +
		"('foo.acme.com','1234','foogroup')," +
		"('bar.acme.com','5678','bargroup')")
	if err != nil {
		t.Error(err)
		return
	}
	_, err = db.Exec("INSERT INTO files(certfp,filename,content) VALUES" +
		"('1234','roadrunner','beep,beep')," +
		"('5678','coyote','ep')")
	if err != nil {
		t.Error(err)
		return
	}
	loadContentForFastSearch(db)
	muxer := createAPImuxer(db, false)
	testAPIcalls(t, muxer, tests)
}

func MockRequest(remoteAddr string, xForwardedFor []string) *http.Request {
	req := &http.Request{
		RemoteAddr: remoteAddr,
		Header:     make(http.Header),
	}
	for _, v := range xForwardedFor {
		req.Header.Add("X-Forwarded-For", v)
	}
	return req
}

func TestGetRealRemoteAddr(t *testing.T) {
	// define tests
	tests := []struct {
		remoteAddr    string
		xForwardedFor []string
		want          string
	}{
		/* We want to test:
		- addresses with/without port numbers
		- addresses in [brackets]
		- one and several elements in X-Forwarded-For
		- RemoteAddr as localhost(loopback) and external
		*/
		{
			remoteAddr:    "127.0.0.1:8080",
			xForwardedFor: []string{"203.0.113.195:41237", "198.51.100.100:38523"},
			want:          "198.51.100.100",
		},
		{
			remoteAddr:    "127.0.0.1",
			xForwardedFor: []string{"203.0.113.195"},
			want:          "203.0.113.195",
		},
		{
			remoteAddr:    "::1",
			xForwardedFor: []string{"2001:db8:85a3:8d3:1319:8a2e:370:7348"},
			want:          "2001:db8:85a3:8d3:1319:8a2e:370:7348",
		},
		{
			remoteAddr:    "::1",
			xForwardedFor: []string{"198.51.100.100:26321", "[2001:db8::1a2b:3c4d]:41237"},
			want:          "2001:db8::1a2b:3c4d",
		},
		{
			remoteAddr:    "[2001:db8::1a2b:3c4d]:1234",
			xForwardedFor: []string{"203.0.113.195:41237", "198.51.100.100:38523"},
			want:          "2001:db8::1a2b:3c4d",
		},
		{
			remoteAddr:    "2001:db8::1a2b:3c4d",
			xForwardedFor: []string{"203.0.113.195:41237", "198.51.100.100:38523"},
			want:          "2001:db8::1a2b:3c4d",
		},
	}
	// perform tests
	for _, test := range tests {
		req := MockRequest(test.remoteAddr, test.xForwardedFor)
		result := getRealRemoteAddr(req).String()
		if result != test.want {
			t.Errorf("Expected getRealRemoteAddr to return %s, got %s", test.want, result)
		}
	}
}
