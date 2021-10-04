package main

// Scan the directory for new files and create tasks for them
import (
	"database/sql"
	"io/ioutil"
	"log"
	"strings"
	"time"
)

type scanQueueDirJob struct{}

func init() {
	RegisterJob(scanQueueDirJob{})
}

func (s scanQueueDirJob) HowOften() time.Duration {
	return time.Second * 10
}

func (s scanQueueDirJob) Run(db *sql.DB) {
	// The "post" cgi script will leave files in this directory.
	const queuedir = "/var/www/nivlheim/queue"

	// Scan the directory for new files and create tasks for them
	files, err := ioutil.ReadDir(queuedir)
	if err != nil {
		log.Panic(err)
	}

	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".meta") {
			// nope.
			continue
		}
		taskurl := "http://nivlheimweb/cgi-bin/processarchive?file=" +
			f.Name()
		// New task
		var err error
		if postgresSupportsOnConflict {
			_, err = db.Exec("INSERT INTO tasks(url) VALUES($1)"+
				" ON CONFLICT DO NOTHING", taskurl)
		} else {
			_, err = db.Exec("INSERT INTO tasks(url) SELECT $1 WHERE "+
				"(SELECT count(*) FROM tasks WHERE url=$1) = 0", taskurl)
		}
		if err != nil {
			log.Println(err.Error())
		}
	}
}
