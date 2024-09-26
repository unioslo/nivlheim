package main

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
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

func doNothing(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "無\n\n") // https://en.wikipedia.org/wiki/Mu_(negative)
	//for k, v := range req.Header {
	//	fmt.Fprintln(w, k+" = "+strings.Join(v, ", "))
	//}
}

// When a machine gets a new certificate to replace the old one,
// the search cache must be updated.
func replaceCertificate(w http.ResponseWriter, req *http.Request) {
	if !isLocal(req) {
		http.Error(w, "Only local requests are allowed", http.StatusForbidden)
		return
	}
	old := req.FormValue("old")
	new := req.FormValue("new")
	if old != "" && new != "" {
		replaceCertificateInCache(old, new)
		http.Error(w, "OK", http.StatusNoContent)
	} else {
		http.Error(w, "Missing/empty parameters: old new", http.StatusBadRequest)
	}
}
