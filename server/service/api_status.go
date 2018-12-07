package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"reflect"
)

type apiMethodStatus struct {
	db *sql.DB
}

func (vars *apiMethodStatus) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != httpGET {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type Status struct {
		NumOfMachines               int                `json:"numberOfMachines"`
		NumOfFiles                  int                `json:"numberOfFiles"`
		ReportingPercentageLastHour int                `json:"reportingPercentageLastHour"`
		IncomingQueueSize           int                `json:"incomingQueueSize"`
		ParseQueueSize              int                `json:"parseQueueSize"`
		TaskQueueSize               int                `json:"taskQueueSize"`
		FailingTasks                int                `json:"failingTasks"`
		AgeOfNewestFile             float32            `json:"ageOfNewestFile"`
		ThroughputPerSecond         float32            `json:"throughputPerSecond"`
		LastExecutionTime           map[string]float32 `json:"lastExecutionTime"`
		Errors                      map[string]string  `json:"errors"`
	}
	status := Status{}

	// Machines last hour
	var machinesLastHour int
	vars.db.QueryRow("SELECT count(*) FROM hostinfo WHERE lastseen > " +
		"now() - interval '1 hour'").Scan(&machinesLastHour)

	// NumOfMachines
	vars.db.QueryRow("SELECT count(*) FROM hostinfo").Scan(&status.NumOfMachines)

	// NumOfFiles
	status.NumOfFiles = numberOfFilesInFastSearch()
	if status.NumOfFiles == -1 {
		// Slower method
		vars.db.QueryRow("SELECT count(*) FROM files WHERE current").Scan(&status.NumOfFiles)
	}

	// ReportingPercentageLastHour
	if status.NumOfMachines > 0 {
		status.ReportingPercentageLastHour = 100 * machinesLastHour / status.NumOfMachines
	} else {
		status.ReportingPercentageLastHour = 0
	}

	// LastExecutionTime
	status.LastExecutionTime = make(map[string]float32, len(jobs))
	status.Errors = make(map[string]string)
	for _, job := range jobs {
		t := reflect.TypeOf(job.job)
		status.LastExecutionTime[t.Name()] = float32(job.lastExecutionTime.Seconds())
		if job.panicObject != nil {
			status.Errors[t.Name()] = fmt.Sprintf("%v", job.panicObject)
		}
	}

	// IncomingQueueSize
	// TODO optimize for large directories
	status.IncomingQueueSize = -1
	const queuedir = "/var/www/nivlheim/queue"
	f, err := os.Open(queuedir)
	if err == nil {
		defer f.Close()
		names, err := f.Readdirnames(0)
		if err == nil {
			status.IncomingQueueSize = len(names) / 2 // half of them are .meta files
		}
	}

	// ParseQueueSize
	vars.db.QueryRow("SELECT count(*) FROM files WHERE NOT parsed").
		Scan(&status.ParseQueueSize)

	// TaskQueueSize
	vars.db.QueryRow("SELECT count(*) FROM tasks").Scan(&status.TaskQueueSize)

	// FailingTasks
	vars.db.QueryRow("SELECT count(*) FROM tasks WHERE status>0").
		Scan(&status.FailingTasks)

	// AgeOfNewestFile
	var t sql.NullFloat64
	status.AgeOfNewestFile = -1
	vars.db.QueryRow("SELECT extract(epoch from now()-received) FROM files " +
		"WHERE parsed ORDER BY fileid DESC LIMIT 1").Scan(&t)
	if t.Valid {
		status.AgeOfNewestFile = float32(t.Float64)
	}

	// ThroughputPerSecond
	status.ThroughputPerSecond = float32(pfib.Sum() / 60.0)

	returnJSON(w, req, status)
}
