package main

import (
	"database/sql"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lib/pq"
)

type Job struct {
	jobid   int64
	url     string
	lasttry time.Time
	status  int
	delay   int
	delay2  int
}

const queuedir = "/var/www/nivlheim/queue"

var quit bool

func main() {
	log.SetFlags(0) // don't print a timestamp

	// handle ctrl-c (SIGINT) and SIGTERM
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		quit = true
		log.Println("Quitting...")
	}()
	defer log.Println("Quit.")
	log.Println("Starting up.")

	db, err := sql.Open("postgres", "dbname=apache sslmode=disable host=/tmp")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	for !quit {
		// Scan the directory for new files and create jobs for them
		scanQueueDir(db)

		// Create jobs to parse new files that have been read into the database
		scanFilesDb(db)

		// Read the current active jobs from the database
		rows, err := db.Query("SELECT jobid, url, lasttry, " +
			"status, delay, delay2 FROM jobs")
		if err != nil {
			log.Fatal(err)
		}
		jobs := make([]Job, 0, 0)
		{
			defer rows.Close()
			for rows.Next() {
				var job Job
				var joburl sql.NullString
				var timestamp pq.NullTime
				err = rows.Scan(&job.jobid, &joburl, &timestamp,
					&job.status, &job.delay, &job.delay2)
				if err != nil {
					log.Fatal(err)
				}
				if joburl.Valid {
					job.url = joburl.String
				}
				if timestamp.Valid {
					job.lasttry = timestamp.Time
				}
				jobs = append(jobs, job)
			}
		}

		// Find jobs that should be run/re-tried right now
		canWait := 20
		for i, job := range jobs {
			if job.lasttry.IsZero() ||
				time.Since(job.lasttry).Seconds() > float64(job.delay) {
				resp, err := http.Get(job.url)
				if err == nil {
					job.status = resp.StatusCode
					resp.Body.Close()
					if resp.StatusCode == 200 || resp.StatusCode == 410 {
						db.Exec("DELETE FROM jobs WHERE jobid=$1", job.jobid)
						// If a job was successful, it might have created
						// more work to do, so, so don't sleep for very long
						canWait = 2
						continue
					}
				} else {
					job.status = 1
				}
				job.lasttry = time.Now()

				// Fibonacci sequence determines the delay in seconds
				if job.delay == 0 && job.delay2 == 0 {
					job.delay = 1
					job.delay2 = 0
				} else {
					newdelay := job.delay + job.delay2
					job.delay2 = job.delay
					job.delay = newdelay
				}
				// Max delay is 24 hours.
				if job.delay > 86400 {
					job.delay = 86400
				}

				if job.delay < canWait {
					canWait = job.delay
				}

				db.Exec("UPDATE jobs SET lasttry=$1, delay=$2, delay2=$3, status=$4 "+
					" WHERE jobid=$5",
					job.lasttry, job.delay, job.delay2, job.status, job.jobid)
				jobs[i] = job
			} else {
				timeleft := job.delay - int(time.Since(job.lasttry).Seconds())
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

func scanQueueDir(db *sql.DB) {
	// Scan the directory for new files and create jobs for them
	files, err := ioutil.ReadDir(queuedir)
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".meta") {
			// nope.
			continue
		}
		joburl := "http://localhost/cgi-bin/processarchive?archivefile=" +
			f.Name()
		job := Job{url: joburl}
		// New job
		db.Exec("INSERT INTO jobs(url) VALUES($1) ON CONFLICT DO NOTHING",
			job.url)
	}
}

func scanFilesDb(db *sql.DB) {
	rows, err := db.Query("SELECT fileid FROM files WHERE parsed = false")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var fileid sql.NullInt64
		rows.Scan(&fileid)
		if fileid.Valid {
			joburl := "http://localhost/cgi-bin/parsefile?fileid=" +
				strconv.FormatInt(fileid.Int64, 10)
			job := Job{url: joburl}
			db.Exec("INSERT INTO jobs(url) VALUES($1) "+
				"ON CONFLICT DO NOTHING", job.url)
		}
	}
}
