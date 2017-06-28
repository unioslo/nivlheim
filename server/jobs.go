package main

import (
	"database/sql"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lib/pq"
)

type Job struct {
	filename string
	lasttry  time.Time
	status   int
	delay    int
	delay2   int
}

const queuedir = "/var/www/nivlheim/queue"

var quit bool = false

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

	db, err := sql.Open("postgres", "sslmode=disable host=/tmp")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	for !quit {
		// Read the current active jobs from the database
		rows, err := db.Query("SELECT filename, lasttry, status, delay, delay2 FROM jobs")
		if err != nil {
			log.Fatal(err)
		}
		jobs := make([]Job, 0, 0)
		{
			defer rows.Close()
			for rows.Next() {
				var job Job
				var timestamp pq.NullTime
				err = rows.Scan(&job.filename, &timestamp, &job.status,
					&job.delay, &job.delay2)
				if err != nil {
					log.Fatal(err)
				}
				if timestamp.Valid {
					job.lasttry = timestamp.Time
				}
				jobs = append(jobs, job)
			}
		}

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
			filename := queuedir + "/" + f.Name()
			for _, j := range jobs {
				if filename == j.filename {
					// Existing job
					filename = ""
					break
				}
			}
			if filename != "" {
				job := Job{filename: filename}
				jobs = append(jobs, job)
				// New job
				db.Exec("INSERT INTO jobs(filename) VALUES($1)", job.filename)
			}
		}

		// Find jobs that should be run/re-tried right now
		canWait := 20
		for i, job := range jobs {
			if job.lasttry.IsZero() ||
				time.Since(job.lasttry).Seconds() > float64(job.delay) {
				if strings.HasPrefix(job.filename, queuedir) {
					basename := job.filename[len(queuedir)+1:]
					url := "http://localhost/cgi-bin/processarchive?archivefile=" +
						url.QueryEscape(basename)
					resp, err := http.Get(url)
					job.status = resp.StatusCode
					if err == nil {
						resp.Body.Close()
						if resp.StatusCode == 200 {
							db.Exec("DELETE FROM jobs WHERE filename=$1", job.filename)
							continue
						}
					}
				}

				job.lasttry = time.Now()

				// Fibonacci sequence
				if job.delay == 0 && job.delay2 == 0 {
					job.delay = 1
					job.delay2 = 0
				} else {
					newdelay := job.delay + job.delay2
					job.delay2 = job.delay
					job.delay = newdelay
				}

				if job.delay < canWait {
					canWait = job.delay
				}

				db.Exec("UPDATE jobs SET lasttry=$1, delay=$2, delay2=$3, status=$4 "+
					" where filename=$5",
					job.lasttry, job.delay, job.delay2, job.status, job.filename)
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
