package main

import (
	"database/sql"
	"log"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"syscall"
	"time"
)

// A Job is an internal piece of code that gets run periodically by this program
type Job interface {
	Run(db *sql.DB)
	HowOften() time.Duration
	//TODO change the HowOften func to a parameter for RegisterJob,
	//     and call it minimumTimeBetweenRuns or something similar.
}

type JobListElement struct {
	job               Job
	lastrun           time.Time
	lastExecutionTime time.Duration
	running, trigger  bool
	panicObject       interface{}
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
		log.Println(err)
		return
	}
	defer db.Close()

	// Determine capabilities of the database
	postgresSupportsOnConflict = false
	var version sql.NullString
	err = db.QueryRow("select version()").Scan(&version)
	if err != nil {
		log.Println(err)
		return
	}
	if version.Valid {
		rePGVersion := regexp.MustCompile("PostgreSQL (\\d+.\\d+.\\d+)")
		mat := rePGVersion.FindStringSubmatch(version.String)
		if len(mat) >= 2 && len(mat[1]) > 0 {
			vstr := mat[1]
			log.Printf("PostgreSQL version: %s", vstr)
			postgresSupportsOnConflict = vstr >= "9.5"
		}
	}

	go runAPI(db, 4040, devmode)
	go taskRunner(db, devmode)

	jobSlots := make(chan bool, 10) // max concurrent running jobs
	for !quit {
		// Run jobs
		for i, j := range jobs {
			if (time.Since(j.lastrun) > j.job.HowOften() || j.trigger) && !j.running {
				jobSlots <- true
				elem := &jobs[i]
				elem.running = true
				elem.lastrun = time.Now()
				elem.trigger = false
				go func() {
					defer func() {
						if r := recover(); r != nil {
							elem.panicObject = r
						}
						elem.lastExecutionTime = time.Since(elem.lastrun)
						elem.lastrun = time.Now()
						elem.running = false
						<-jobSlots
					}()
					elem.job.Run(db)
				}()
			}
		}

		// Sleep
		time.Sleep(time.Second)
	}
	// wait for jobs to finish
	log.Println("Waiting for running jobs to finish...")
	left := cap(jobSlots)
	start := time.Now()
	for left > 0 && time.Since(start) <= time.Second*10 {
		select {
		case jobSlots <- true:
			left--
		default:
			time.Sleep(time.Second)
		}
	}
}

func triggerJob(job Job) {
	for i, jobitem := range jobs {
		if reflect.TypeOf(jobitem.job) == reflect.TypeOf(job) {
			jobs[i].trigger = true
			return
		}
	}
	panic("Trying to trigger an unregistered job?")
}
