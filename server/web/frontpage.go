package main

import (
	"database/sql"
	"html/template"
	"net/http"
	"net/http/cgi"
	"os"

	"github.com/lib/pq"
)

var templatePath string
var templates *template.Template
var dbConnectionString string

type WaitingForApproval struct {
	Ipaddr   sql.NullString
	Hostname sql.NullString
	Received pq.NullTime
}

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

	if req.FormValue("approve") != "" {
		approved := req.FormValue("approve") == "1"
		res, err := db.Exec("UPDATE waiting_for_approval SET approved=$1 "+
			"WHERE hostname=$2 AND ipaddr=$3",
			approved,
			req.FormValue("h"),
			req.FormValue("ip"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rows, err := res.RowsAffected()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rows == 0 {
			http.Error(w, "Record not found.", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		switch approved {
		case true:
			w.Write([]byte("Approved"))
		case false:
			w.Write([]byte("Denied"))
		}
		return
	}

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

	approval := make([]WaitingForApproval, 0, 0)
	rows, err = db.Query("SELECT ipaddr, hostname, received " +
		"FROM waiting_for_approval WHERE approved IS NULL ORDER BY hostname")
	if err != nil {
		http.Error(w, "1: "+err.Error(), http.StatusInternalServerError)
		return
	} else {
		defer rows.Close()
		for rows.Next() {
			var app WaitingForApproval
			err = rows.Scan(&app.Ipaddr, &app.Hostname, &app.Received)
			if err != nil && err != sql.ErrNoRows {
				http.Error(w, "4: "+err.Error(), http.StatusInternalServerError)
				return
			}
			approval = append(approval, app)
		}
	}

	// Fill template values
	tValues := make(map[string]interface{})
	tValues["machines"] = machines
	tValues["filesLastHour"] = filesLastHour
	tValues["totalMachines"] = totalMachines
	if totalMachines > 0 {
		tValues["reportingPercentage"] = (machinesLastHour * 100) / totalMachines
	}
	tValues["approval"] = approval

	// Render template
	templates.ExecuteTemplate(w, "frontpage.html", tValues)
}

func staticfiles(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "../static/"+req.URL.Path[8:])
}
