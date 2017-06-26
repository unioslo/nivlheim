package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	fmt.Printf("Hei verden!\n")
	db, err := sql.Open("postgres", "")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	for {
		// Every 10s or so:
		// scan the directory for new files
		// insert job rows for any new files
		// load jobs from the table
		// compute which ones should be retried right now
		// try each job, and update or delete the corresponding row
		// go to sleep
		time.Sleep(time.Second * 10)
	}
}
