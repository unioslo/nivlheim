package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/lib/pq"
)

type Job struct {
	filename string
	lasttry time.Time
	status int
	delay int
	delay2 int
}

const queuedir = "/var/www/nivlheim/queue"

var quit bool = false

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func(){
	    for range c {
	        // sig is a ^C, handle it
			quit = true
			fmt.Println("Quitting...")
	    }
	}()
	defer fmt.Println("Quit.")

	/*
	location, err := time.LoadLocation("Europe/Oslo")
	if err != nil {
		log.Fatal(err)
	}
	*/

	db, err := sql.Open("postgres", "sslmode=disable host=/tmp")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	for !quit {
		// Read the current active jobs from the database
		rows, err := db.Query("SELECT filename, lasttry, status, delay, delay2 FROM jobs");
		if err != nil {
			log.Fatal(err)
		}
		jobs := make([]Job, 0, 0)
		{
			defer rows.Close()
			for ; rows.Next(); {
				var job Job
				var timestamp pq.NullTime
				err = rows.Scan(&job.filename, &timestamp, &job.status,
					&job.delay, &job.delay2)
				if err != nil {
					log.Fatal(err)
				}
				if timestamp.Valid { job.lasttry = timestamp.Time }
				jobs = append(jobs, job)
			}
		}

		// Scan the directory for new files and create jobs for them
		files, err := ioutil.ReadDir(queuedir)
		if err != nil {
			log.Fatal(err)
		}
		for _,f := range files {
			if strings.HasSuffix(f.Name(), ".meta") {
				// nope.
				continue
			}
			filename := queuedir + "/" + f.Name()
			for _,j := range jobs {
				if filename == j.filename {
					fmt.Printf("Existing job: %s\n", filename)
					filename = ""
					break
				}
			}
			if filename != "" {
				job := Job{filename:filename}
				jobs = append(jobs, job)
				fmt.Printf("New job: %s\n", job.filename)
				db.Exec("INSERT INTO jobs(filename) VALUES($1)", job.filename)
			}
		}

		// Find jobs that should be run/re-tried right now
		canWait := 20
		for i, job := range jobs {
			if job.lasttry.IsZero() || 
			time.Since(job.lasttry).Seconds() > float64(job.delay) {
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

				fmt.Printf("Tried job. Delay: %d s\n", job.delay)
				if job.delay < canWait {
					canWait = job.delay
				}

				db.Exec("UPDATE jobs SET lasttry=$1, delay=$2, delay2=$3 "+
					" where filename=$4",
					job.lasttry, job.delay, job.delay2, job.filename)
				jobs[i] = job
			} else {
				timeleft := job.delay - int(time.Since(job.lasttry).Seconds())
				if timeleft < canWait {
					canWait = timeleft
				}
			}
		}

		// Sleep
		fmt.Printf("Sleeping for %d seconds.\n", canWait)
		for second := 0; second < canWait && !quit; second++ {
			time.Sleep(time.Second)
		}
	}
}
