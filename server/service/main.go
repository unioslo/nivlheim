package main

import (
	"database/sql"
	"log"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"
)

// A Job is an internal piece of code that gets run periodically by this program
type Job interface {
	Run(db *sql.DB)
	HowOften() time.Duration
	//TODO gjør om HowOften denne til en parameter til RegisterJob,
	//     og kall den minimumTimeBetweenRuns eller noe.
}

type JobListElement struct {
	job               Job
	lastrun           time.Time
	lastExecutionTime time.Duration
	running           bool
}

func RegisterJob(newjob Job) {
	jobs = append(jobs, JobListElement{job: newjob})
}

var jobs []JobListElement
var postgresSupportsOnConflict bool

func main() {
	log.SetFlags(0) // don't print a timestamp
	devmode := len(os.Args) >= 2 && os.Args[1] == "--dev"

	// handle ctrl-c (SIGINT) and SIGTERM
	var quit bool
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		quit = true
		log.Println("\rShutting down...")
	}()
	defer log.Println("Stopped.")
	log.Println("Starting up.")

	if devmode {
		log.Println("Running in development mode.")
	}

	// Connect to database
	var dbConnectionString string
	if devmode {
		//dbConnectionString = "sslmode=disable host=/var/run/postgresql"
		dbConnectionString = "sslmode=disable dbname=apache user=apache host=nivlheim-beta.uio.no"
	} else {
		dbConnectionString = "dbname=apache host=/var/run/postgresql"
	}
	db, err := sql.Open("postgres", dbConnectionString)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Determine capabilities of the database
	var rePGVersion = regexp.MustCompile("PostgreSQL (\\d+.\\d+.\\d+)")
	postgresSupportsOnConflict = false
	rows, err := db.Query("select version()")
	if err != nil {
		log.Fatal(err)
	} else {
		defer rows.Close()
		if rows.Next() {
			var version sql.NullString
			err = rows.Scan(&version)
			if err == nil {
				if version.Valid {
					mat := rePGVersion.FindStringSubmatch(version.String)
					if len(mat) >= 2 && len(mat[1]) > 0 {
						vstr := mat[1]
						log.Printf("PostgreSQL version: %s", vstr)
						postgresSupportsOnConflict = vstr >= "9.5"
					}
				}
			}
		}
	}

	go runAPI(db, 4040, devmode)
	go taskRunner(db, devmode)

	jobSlots := make(chan bool, 10) // max concurrent running jobs
	for !quit {
		// Run jobs
		for i, j := range jobs {
			if time.Since(j.lastrun) > j.job.HowOften() && !j.running {
				jobSlots <- true
				elem := &jobs[i]
				elem.running = true
				elem.lastrun = time.Now()
				go func() {
					defer func() { <-jobSlots }()
					elem.job.Run(db)
					elem.lastExecutionTime = time.Since(elem.lastrun)
					elem.lastrun = time.Now()
					elem.running = false
				}()
			}
		}

		// Sleep
		time.Sleep(time.Second)
	}
	// wait for jobs to finish
	log.Println("Waiting for running jobs to finish...")
	for i := 0; i < cap(jobSlots); i++ {
		jobSlots <- true
	}
}
