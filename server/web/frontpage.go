package main

import (
	"database/sql"
	"html/template"
	"net/http"
	"net/http/cgi"
	"os"

	_ "github.com/lib/pq"
)

var templatePath string
var templates *template.Template
var dbConnectionString string

func init() {
	http.HandleFunc("/", frontpage)
	http.HandleFunc("/search", search)
	http.HandleFunc("/browse", browse)
}

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "--dev" {
		templatePath = "../templates"
		dbConnectionString = "host=potetgull.mooo.com " +
			"dbname=apache sslmode=disable user=apache"
		http.HandleFunc("/static/", staticfiles)
		http.ListenAndServe(":8080", nil)
	} else {
		templatePath = "/var/www/nivlheim/templates"
		dbConnectionString = "dbname=apache host=/var/run/postgresql"
		cgi.Serve(nil)
	}
}

func frontpage(w http.ResponseWriter, req *http.Request) {
	db, err := sql.Open("postgres", dbConnectionString)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Load html templates
	templates, err := template.ParseGlob(templatePath + "/*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	machines := make([]string, 0, 0)
	rows, err := db.Query("SELECT hostname FROM hostinfo ORDER BY hostname")
	if err != nil {
		http.Error(w, "1: "+err.Error(), http.StatusInternalServerError)
		return
	} else {
		defer rows.Close()
		for rows.Next() {
			var hostname sql.NullString
			err = rows.Scan(&hostname)
			if err != nil && err != sql.ErrNoRows {
				http.Error(w, "2: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if hostname.Valid {
				machines = append(machines, hostname.String)
			}
		}
	}

	var filesLastHour int
	db.QueryRow("SELECT count(*) FROM files WHERE " +
		"received > now() - '1 hour'::INTERVAL").Scan(&filesLastHour)

	var machinesLastHour int
	db.QueryRow("SELECT count(distinct certfp) FROM files WHERE " +
		"received > now() - '1 hour'::INTERVAL").Scan(&machinesLastHour)

	var totalMachines int
	db.QueryRow("SELECT count(*) FROM hostinfo").Scan(&totalMachines)

	// Fill template values
	tValues := make(map[string]interface{})
	tValues["machines"] = machines
	tValues["filesLastHour"] = filesLastHour
	tValues["totalMachines"] = totalMachines
	tValues["reportingPercentage"] = (machinesLastHour * 100) / totalMachines

	// Render template
	templates.ExecuteTemplate(w, "frontpage.html", tValues)
}

func staticfiles(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "../static/"+req.URL.Path[8:])
}
