package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"nivlheim/utility"
	"strings"
	"testing"
)

type apiCall struct {
	methodAndPath, body string
	remoteAddr          string
	expectStatus        int
	expectJSON          string
	expectContent       string
	accessProfile       *AccessProfile
	sessionProfile      *AccessProfile
	runAsNotAuth        bool
}

func testAPIcalls(t *testing.T, mux *http.ServeMux, tests []apiCall) {
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
		req.RemoteAddr = "123.123.123.123"
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		if tt.runAsNotAuth {
			// Enable auth, but don't supply any auth or session information.
			config.AuthRequired = true
		} else if tt.accessProfile != nil {
			// Enable auth
			config.AuthRequired = true
			// Create an API key
			key := GenerateTemporaryAPIKey(tt.accessProfile)
			// Set the Authorization http header
			req.Header.Add("Authorization", "APIKEY "+string(key))
		} else if tt.sessionProfile != nil {
			// Enable auth
			config.AuthRequired = true
			// Fake a session
			trapResponse := httptest.NewRecorder()
			session := newSession(trapResponse, req)
			// Copy the cookie from the response to the request
			response := trapResponse.Result()
			req.AddCookie(response.Cookies()[0])
			// Set the access profile in the session
			session.AccessProfile = tt.sessionProfile
			session.userinfo.IsAdmin = tt.sessionProfile.IsAdmin()
			// Because we're faking a session, we also need to fake the headers Origin and Host,
			// otherwise we'll trip the CSRF protection
			req.Header.Add("Origin", "http://www.acme.com/")
			req.Host = "www.acme.com"
		} else {
			// Disable auth. This will bypass authentication and authorization,
			// effectively running as admin. This is the default when testing API calls.
			config.AuthRequired = false
		}
		if tt.remoteAddr != "" {
			req.RemoteAddr = tt.remoteAddr
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if status := rr.Code; status != tt.expectStatus {
			apstr := ""
			if tt.sessionProfile != nil {
				apstr = fmt.Sprintf("\nSession access: %#v", tt.sessionProfile)
			}
			if tt.accessProfile != nil {
				apstr = fmt.Sprintf("\nKey access: %#v", tt.accessProfile)
			}
			t.Errorf("%s\nreturned status %v, expected %v.%s\n%s",
				tt.methodAndPath, status, tt.expectStatus,
				apstr, rr.Body.String())
			continue
		}
		if tt.expectJSON != "" {
			isEqual, err := utility.IsEqualJSON(rr.Body.String(), tt.expectJSON)
			if err != nil {
				t.Error(err)
			}
			if !isEqual {
				t.Errorf("%s\nGot result %s,\nexpected %s",
					tt.methodAndPath,
					rr.Body.String(),
					tt.expectJSON)
			}
		}
		if tt.expectContent != "" {
			if !strings.Contains(rr.Body.String(), tt.expectContent) {
				t.Errorf("%s\nGot result %s,\nexpected something containing %s",
					tt.methodAndPath,
					rr.Body.String(),
					tt.expectContent)
			}
		}
	}
}
