package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/lib/pq"
)

func runAPI(theDB *sql.DB, port int, devmode bool) {
	mux := http.NewServeMux()
	mux.Handle("/api/v0/awaitingApproval", &apiMethodAwaitingApproval{db: theDB})
	mux.Handle("/api/v0/awaitingApproval/", &apiMethodAwaitingApproval{db: theDB})
	mux.Handle("/api/v0/file", &apiMethodFile{db: theDB})
	mux.Handle("/api/v0/host", &apiMethodHost{db: theDB})
	mux.Handle("/api/v0/hostlist", &apiMethodHostList{db: theDB, devmode: devmode})
	mux.Handle("/api/v0/searchpage", &apiMethodSearchPage{db: theDB})
	mux.Handle("/api/v0/status", &apiMethodStatus{db: theDB})
	var h http.Handler = mux
	if devmode {
		h = wrapLog(wrapAllowLocalhostCORS(h))
	}
	log.Printf("Serving API requests on localhost:%d\n", port)
	err := http.ListenAndServe(fmt.Sprintf("localhost:%d", port), h)
	if err != nil {
		log.Println(err.Error())
	}
}

// returnJSON marshals the given object and writes it as the response,
// and also sets the proper Content-Type header.
// Remember to return after calling this function.
func returnJSON(w http.ResponseWriter, req *http.Request, data interface{}) {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println(err.Error())
		return
	}
	bytes = append(bytes, 0xA) // end with a line feed, because I'm a nice person
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(bytes)
}

// For requests originating from localhost (typically on another port),
// this wrapper adds http headers that allow that origin.
// This makes it easier to test locally when developing.
func wrapAllowLocalhostCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		match, err := regexp.MatchString("http://(127\\.0\\.0\\.1|localhost)",
			req.Header.Get("Origin"))
		if match {
			w.Header().Set("Access-Control-Allow-Origin", req.Header.Get("Origin"))
			w.Header().Set("Access-Control-Allow-Methods",
				"GET, POST, HEAD, OPTIONS, PUT, DELETE, PATCH")
			w.Header().Set("Vary", "Origin")
		} else if err != nil {
			log.Println(err)
		}
		if req.Method == "OPTIONS" {
			// When cross-domain, browsers sends OPTIONS first, to check for CORS headers
			// See: https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS
			http.Error(w, "", http.StatusNoContent) // 204 OK
			return
		}
		h.ServeHTTP(w, req)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func wrapLog(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		lrw := &loggingResponseWriter{w, http.StatusOK}
		h.ServeHTTP(lrw, req)
		log.Printf("[%d] %s %s\n", lrw.statusCode, req.Method, req.URL)
	})
}

// Wrappers for sql nulltypes that encodes the values when marshalling JSON
type jsonTime pq.NullTime
type jsonString sql.NullString

func (jst jsonTime) MarshalJSON() ([]byte, error) {
	if jst.Valid && !jst.Time.IsZero() {
		return []byte(fmt.Sprintf("\"%s\"", jst.Time.Format(time.RFC3339))), nil
	}
	return []byte("null"), nil
}

func (ns jsonString) MarshalJSON() ([]byte, error) {
	if ns.Valid {
		return json.Marshal(ns.String)
	}
	return []byte("null"), nil
}

type httpError struct {
	message string
	code    int
}

func unpackFieldParam(fieldParam string, allowedFields []string) (map[string]bool, *httpError) {
	if fieldParam == "" {
		return nil, &httpError{
			message: "Missing or empty parameter: fields",
			code:    http.StatusUnprocessableEntity,
		}
	}
	fields := make(map[string]bool)
	for _, f := range strings.Split(fieldParam, ",") {
		ok := false
		for _, af := range allowedFields {
			if strings.EqualFold(f, af) {
				ok = true
				fields[af] = true
				break
			}
		}
		if !ok {
			return nil, &httpError{
				message: "Unsupported field name: " + f,
				code:    http.StatusUnprocessableEntity,
			}
		}
	}
	return fields, nil
}

func contains(needle string, haystack []string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
