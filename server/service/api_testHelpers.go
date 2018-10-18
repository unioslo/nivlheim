package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usit-gd/nivlheim/server/service/utility"
)

type apiCall struct {
	methodAndPath, body string
	expectStatus        int
	expectJSON          string
	expectContent       string
	accessProfile       *AccessProfile
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
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		if tt.runAsNotAuth {
			// If the request isn't coming from localhost, the auth wrappers will run as normally
			req.RemoteAddr = "123.123.123.123"
		} else if tt.accessProfile != nil {
			// Fake a session
			trapResponse := httptest.NewRecorder()
			req.RemoteAddr = "111.222.111.222"
			session := newSession(trapResponse, req)
			// Copy the cookie from the response to the request
			response := trapResponse.Result()
			req.AddCookie(response.Cookies()[0])
			// Set the access profile in the session
			session.AccessProfile = tt.accessProfile
			session.userinfo.IsAdmin = tt.accessProfile.IsAdmin()
			// Because we're faking a session, we also need to fake the headers Origin and Host,
			// otherwise we'll trip the CSRF protection
			req.Header.Add("Origin", "http://www.acme.com/")
			req.Host = "www.acme.com"
		} else {
			// Coming from localhost will bypass authentication and authorization,
			// effectively running as admin. This is the default when testing API calls.
			req.RemoteAddr = "127.0.0.1"
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if status := rr.Code; status != tt.expectStatus {
			apstr := ""
			if tt.accessProfile != nil {
				apstr = fmt.Sprintf("\n%#v", tt.accessProfile)
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
