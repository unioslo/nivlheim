package main

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

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
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "無\n\n") // https://en.wikipedia.org/wiki/Mu_(negative)
}
