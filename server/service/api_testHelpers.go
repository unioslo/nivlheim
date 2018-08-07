package main

import (
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
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if status := rr.Code; status != tt.expectStatus {
			t.Errorf("%s\nreturned status %v, expected %v.\n%s",
				tt.methodAndPath, status, tt.expectStatus,
				rr.Body.String())
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
	}
}
