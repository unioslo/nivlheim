package main

// Create tasks to parse new files that have been read into the database
import (
	"database/sql"
	"log"
	"strconv"
	"time"
)

type scanFilesTableJob struct{}

func init() {
	RegisterJob(scanFilesTableJob{})
}

func (s scanFilesTableJob) HowOften() time.Duration {
	return 0
}

func (s scanFilesTableJob) Run(db *sql.DB) {
	rows, err := db.Query("SELECT fileid FROM files WHERE parsed = false")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var fileid sql.NullInt64
		rows.Scan(&fileid)
		if fileid.Valid {
			taskurl := "http://localhost/cgi-bin/parsefile?fileid=" +
				strconv.FormatInt(fileid.Int64, 10)
			task := Task{url: taskurl}
			db.Exec("INSERT INTO tasks(url) VALUES($1) "+
				"ON CONFLICT DO NOTHING", task.url)
		}
	}
}
