package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

const (
	httpGET    = "GET"
	httpPOST   = "POST"
	httpPUT    = "PUT"
	httpDELETE = "DELETE"
)

func runAPI(theDB *sql.DB, port int, devmode bool) {
	mux := http.NewServeMux()

	// API functions
	api := http.NewServeMux()
	api.Handle("/api/v0/awaitingApproval", &apiMethodAwaitingApproval{db: theDB})
	api.Handle("/api/v0/awaitingApproval/", &apiMethodAwaitingApproval{db: theDB})
	api.Handle("/api/v0/file", &apiMethodFile{db: theDB})
	api.Handle("/api/v0/host", &apiMethodHost{db: theDB})
	api.Handle("/api/v0/hostlist", &apiMethodHostList{db: theDB, devmode: devmode})
	api.Handle("/api/v0/searchpage", &apiMethodSearchPage{db: theDB, devmode: devmode})
	api.Handle("/api/v0/settings/ipranges", &apiMethodIpRanges{db: theDB})
	api.Handle("/api/v0/settings/ipranges/", &apiMethodIpRanges{db: theDB})
	api.Handle("/api/v0/settings/", &apiMethodSettings{db: theDB})
	api.Handle("/api/v0/settings", &apiMethodSettings{db: theDB})
	api.Handle("/api/v0/settings/customfields", &apiMethodCustomFieldsCollection{db: theDB})
	api.Handle("/api/v0/settings/customfields/", &apiMethodCustomFieldsItem{db: theDB})
	api.Handle("/api/v0/status", &apiMethodStatus{db: theDB})
	api.HandleFunc("/api/v0/userinfo", apiGetUserInfo)
	var h http.Handler = api
	h = wrapCSRFprotection(h)
	if !devmode {
		h = wrapLog(h)
	}
	mux.Handle("/api/v0/", h)

	// Oauth2-related endpoints
	mux.HandleFunc("/api/oauth2/start", startOauth2Login)
	mux.HandleFunc("/api/oauth2/redirect", handleOauth2Redirect)
	mux.HandleFunc("/api/oauth2/logout", oauth2Logout)

	// internal API functions. Only allowed from localhost.
	internal := http.NewServeMux()
	internal.HandleFunc("/api/internal/triggerJob/", runJob)
	internal.HandleFunc("/api/internal/unsetCurrent", unsetCurrent)
	internal.HandleFunc("/api/internal/countFiles", countFiles)
	h = internal
	h = wrapOnlyAllowLocal(h)
	mux.Handle("/api/internal/", h)

	//
	mux.HandleFunc("/api/v0/mu", doNothing)

	h = mux
	if devmode {
		// In development mode, log every request to stdout, and
		// add CORS headers to responses to local requests.
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

func isLocal(req *http.Request) bool {
	// The X-Forwarded-For header can be set by the client,
	// so just to be safe let's not trust any proxy connections.
	if req.Header.Get("X-Forwarded-For") != "" {
		return false
	}
	return strings.HasPrefix(req.RemoteAddr, "127.0.0.1")
}

func wrapOnlyAllowLocal(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !isLocal(req) {
			http.Error(w, "Only local requests are allowed", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, req)
	})
}

func wrapCSRFprotection(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Requests from localhost are allowed regardless
		if isLocal(req) {
			h.ServeHTTP(w, req)
			return
		}

		// First, find the hostname for this request.
		// Apache httpd ProxyPass will add the X-Forwarded-Host header.
		// It will contain more than one (comma-separated) value if the original
		// request already contained a X-Forwarded-Host header.
		var hostlist []string
		fwdh := req.Header.Get("X-Forwarded-Host")
		if fwdh != "" {
			hostlist = strings.Split(fwdh, ",")
		} else {
			// If X-Forwarded-Host is absent, try using the regular host header
			host := req.Host
			// It may be of the form "host:port"
			if i := strings.Index(host, ":"); i > -1 {
				host = host[0:i]
			}
			hostlist = []string{host}
		}

		// If the http header "Origin" is present, check that it matches
		// the host name in the request
		origin := req.Header.Get("Origin")
		if origin != "" {
			u, err := url.Parse(origin)
			if err != nil {
				http.Error(w, "", http.StatusBadRequest)
				return
			}
			origin = u.Hostname()
			found := false
			for _, h := range hostlist {
				if h == origin {
					found = true
					break
				}
			}
			if !found {
				// Origin is present and doesn't match the host
				http.Error(w, "CSRF", http.StatusBadRequest)
				return
			}
		}

		// If the http header "Referer" is present, check that it matches
		// the host name in the request
		referer := req.Header.Get("Referer")
		if referer != "" {
			u, err := url.Parse(referer)
			if err != nil {
				http.Error(w, "", http.StatusBadRequest)
				return
			}
			referer = u.Hostname()
			found := false
			for _, h := range hostlist {
				if h == referer {
					found = true
					break
				}
			}
			if !found {
				// Referer is present and doesn't match the host
				http.Error(w, "CSRF", http.StatusBadRequest)
				return
			}
		}

		// If neither referer nor origin headers are present,
		// the assumption is that this request doesn't come from a web browser.
		// In that case, there shouldn't be any session cookie in the request either.
		if req.Header.Get("Referer") == "" && req.Header.Get("Origin") == "" && HasSessionCookie(req) {
			http.Error(w, "CSRF", http.StatusBadRequest)
			return
		}

		h.ServeHTTP(w, req)
		return
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

// unpackFieldParam is a helper function to parse a comma-separated
// "fields" parameter and verify that the given fields are valid.
func unpackFieldParam(fieldParam string, allowedFields []string) (map[string]bool, *httpError) {
	if fieldParam == "" {
		return nil, &httpError{
			message: "Missing or empty parameter: fields",
			code:    http.StatusUnprocessableEntity,
		}
	}
	fields := make(map[string]bool)
	for _, f := range strings.Split(fieldParam, ",") {
		if len(f) == 0 {
			continue
		}
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

// runJob sets the "trigger" flag on the Job struct in the jobs array,
// but it doesn't actually execute the job. The main loop does that.
func runJob(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLocal(req) {
		http.Error(w, "Only local requests are allowed", http.StatusForbidden)
		return
	}
	match := regexp.MustCompile("/(\\w+)$").FindStringSubmatch(req.URL.Path)
	if match == nil {
		http.Error(w, "Missing job name in URL path", http.StatusUnprocessableEntity)
		return
	}
	for i, jobitem := range jobs {
		t := reflect.TypeOf(jobitem.job)
		if t.Name() == match[1] {
			// this will make main run the job
			jobs[i].trigger = true
			http.Error(w, "OK", http.StatusNoContent)
			return
		}
	}
	http.Error(w, "Job not found.", http.StatusNotFound)
}

// unsetCurrent is an internal API function that the CGI scripts use
// to notify the system service/daemon that some file(s) have had
// their "current" flag cleared, and can be removed from the
// in-memory search cache.
func unsetCurrent(w http.ResponseWriter, req *http.Request) {
	if !isLocal(req) {
		http.Error(w, "Only local requests are allowed", http.StatusForbidden)
		return
	}
	for _, s := range strings.Split(req.FormValue("ids"), ",") {
		fileID, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			removeFileFromFastSearch(fileID)
		}
	}
	http.Error(w, "OK", http.StatusNoContent)
}

// countFiles is an internal API function that the CGI scripts use
// to notify the system service/daemon that a number of files
// have been processed, so we can produce an accurate count of
// files-per-minute.
func countFiles(w http.ResponseWriter, req *http.Request) {
	if !isLocal(req) {
		http.Error(w, "Only local requests are allowed", http.StatusForbidden)
		return
	}
	i, err := strconv.Atoi(req.FormValue("n"))
	if err != nil || i == 0 {
		return
	}
	pfib.Add(float64(i)) // pfib = parsed files interval buffer
}

func doNothing(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", req.Header.Get("Origin"))
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Vary", "Origin")
	if req.Method == "OPTIONS" || req.Method == "HEAD" {
		// When cross-domain, browsers sends OPTIONS first, to check for CORS headers
		// See: https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS
		http.Error(w, "", http.StatusNoContent) // 204 OK
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "無\n\n") // https://en.wikipedia.org/wiki/Mu_(negative)
	for key, values := range req.Header {
		fmt.Fprintf(w, "%s = %v\n", key, values)
	}
}

func isTrueish(s string) bool {
	s = strings.ToLower(s)
	return s == "1" || s == "t" || s == "true" || s == "yes" || s == "y"
}

// QueryList performs a database query and returns a slice of maps.
// For convenience, the slice can be passed directly to returnJSON.
func QueryList(db *sql.DB, statement string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := db.Query(statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		// Source: https://kylewbanks.com/blog/query-result-to-map-in-golang

		// Create a slice of interface{}'s to represent each column,
		// and a second slice to contain pointers to each item in the columns slice.
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		// Scan the result into the column pointers...
		if err := rows.Scan(columnPointers...); err != nil {
			return nil, err
		}

		// Create our map, and retrieve the value for each column from the pointers slice,
		// storing it in the map with the name of the column as the key.
		m := make(map[string]interface{})
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			m[colName] = *val
		}

		result = append(result, m)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return result, nil
}

// QueryColumn performs a database query that is expected to return one column,
// and returns a slice with the values.
// For convenience, the slice can be passed directly to returnJSON.
func QueryColumn(db *sql.DB, statement string, args ...interface{}) ([]interface{}, error) {
	rows, err := db.Query(statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]interface{}, 0)
	for rows.Next() {
		var v interface{}
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return result, nil
}
