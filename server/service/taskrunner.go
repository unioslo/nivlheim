package main

// This program acts as a task queue manager.

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/lib/pq"
)

// Task is a struct that holds an entry in a task queue.
// The queue itself is stored in a database table.
// Other parts of the system can create tasks by inserting rows.
// A task will be retried until it succeeds, and then discarded.
// A task has an url. To run a task, a http get request will be performed.
// A return status of 200 is interpreted as success, everything else is a failure.
// In case of failure, the task will be retried after a while.
// An exception is http status 410 which is interpreted as a permanent failure
// that is pointless to retry.
type Task struct {
	taskid  int64
	url     string
	lasttry time.Time
	status  int
	delay   int
	delay2  int
}

// A Job is an internal piece of code that gets run periodically by this program
type Job interface {
	Run(db *sql.DB)
	HowOften() time.Duration
	//TODO gjør om HowOften denne til en parameter til RegisterJob,
	//     og kall den minimumTimeBetweenRuns eller noe.
}

type JobListElement struct {
	job     Job
	lastrun time.Time
}

func RegisterJob(newjob Job) {
	jobs = append(jobs, JobListElement{job: newjob})
}

var jobs []JobListElement
var quit bool
var postgresSupportsOnConflict bool

func main() {
	log.SetFlags(0) // don't print a timestamp
	devmode := len(os.Args) >= 2 && os.Args[1] == "--dev"

	// handle ctrl-c (SIGINT) and SIGTERM
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
		dbConnectionString = "sslmode=disable host=/var/run/postgresql"
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

	for !quit {
		// Run jobs
		for _, j := range jobs {
			if time.Since(j.lastrun) > j.job.HowOften() {
				j.job.Run(db)
				j.lastrun = time.Now()
			}
		}

		// Read the current active tasks from the database
		rows, err := db.Query("SELECT taskid, url, lasttry, " +
			"status, delay, delay2 FROM tasks")
		if err != nil {
			log.Fatal(err)
		}
		tasks := make([]Task, 0, 0)
		{
			defer rows.Close()
			for rows.Next() {
				var task Task
				var taskurl sql.NullString
				var timestamp pq.NullTime
				err = rows.Scan(&task.taskid, &taskurl, &timestamp,
					&task.status, &task.delay, &task.delay2)
				if err != nil {
					log.Fatal(err)
				}
				if taskurl.Valid {
					task.url = taskurl.String
				}
				if timestamp.Valid {
					task.lasttry = timestamp.Time
				}
				tasks = append(tasks, task)
			}
		}

		// Find tasks that should be run/re-tried right now
		canWait := 20
		for i, task := range tasks {
			if task.lasttry.IsZero() ||
				time.Since(task.lasttry).Seconds() > float64(task.delay) {
				resp, err := http.Get(task.url)
				if err == nil {
					task.status = resp.StatusCode
					resp.Body.Close()
					if resp.StatusCode == 200 || resp.StatusCode == 410 {
						db.Exec("DELETE FROM tasks WHERE taskid=$1", task.taskid)
						// If a task was successful, it might have created
						// new tasks, so, so don't sleep for very long
						canWait = 2
						continue
					}
				} else {
					task.status = 1
				}
				task.lasttry = time.Now()

				// Fibonacci sequence determines the delay in seconds
				if task.delay == 0 && task.delay2 == 0 {
					task.delay = 1
					task.delay2 = 0
				} else {
					newdelay := task.delay + task.delay2
					task.delay2 = task.delay
					task.delay = newdelay
				}
				// Max delay is 24 hours.
				if task.delay > 86400 {
					task.delay = 86400
				}

				if task.delay < canWait {
					canWait = task.delay
				}

				db.Exec("UPDATE tasks SET lasttry=$1, delay=$2, delay2=$3, status=$4 "+
					" WHERE taskid=$5",
					task.lasttry, task.delay, task.delay2, task.status, task.taskid)
				tasks[i] = task
			} else {
				timeleft := task.delay - int(time.Since(task.lasttry).Seconds())
				if timeleft < canWait {
					canWait = timeleft
				}
			}
		}

		// Sleep
		for second := 0; second < canWait && !quit; second++ {
			time.Sleep(time.Second)
		}
	}
}
