package main

import (
	"database/sql"
	"net/http"
)

type apiMethodStatus struct {
	db *sql.DB
}

func (vars *apiMethodStatus) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	type Status struct {
		FilesLastHour               int `json:"filesLastHour"`
		NumOfMachines               int `json:"numberOfMachines"`
		ReportingPercentageLastHour int `json:"reportingPercentageLastHour"`
	}
	status := Status{}

	var machinesLastHour int

	err := vars.db.QueryRow("SELECT count(*), count(distinct(certfp)) "+
		"FROM files WHERE received > now() - interval '1 hour'").
		Scan(&status.FilesLastHour, &machinesLastHour)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = vars.db.QueryRow("SELECT count(*) FROM hostinfo").
		Scan(&status.NumOfMachines)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if status.NumOfMachines > 0 {
		status.ReportingPercentageLastHour = 100 * machinesLastHour / status.NumOfMachines
	} else {
		status.ReportingPercentageLastHour = 0
	}

	returnJSON(w, req, status)
}
