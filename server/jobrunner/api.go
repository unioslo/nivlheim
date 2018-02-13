package main

// Create tasks to parse new files that have been read into the database
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func runAPI(theDB *sql.DB, port int) {
	http.Handle("/api/v0/awaitingApproval", &apiMethodAwaitingApproval{db: theDB})
	http.Handle("/api/v0/file", &apiMethodFile{db: theDB})
	log.Printf("Serving API requests on localhost:%d\n", port)
	log.Println(http.ListenAndServe(fmt.Sprintf("localhost:%d", port), nil))
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
	// If originating from localhost (typically on another port),
	// allow that origin. This makes development easier.
	if strings.Index(req.Header.Get("Origin"), "http://127.0.0.1") == 0 {
		w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:8000")
	}
	w.Write(bytes)
}

type jsonTime time.Time

func (jst jsonTime) MarshalJSON() ([]byte, error) {
	tt := time.Time(jst)
	if tt.IsZero() {
		return []byte("\"\""), nil
	}
	return []byte(fmt.Sprintf("\"%s\"", tt.Format(time.RFC3339))), nil
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
				fields[strings.ToLower(f)] = true
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
