package main

import (
	"database/sql"
	"log"
	"time"

	"github.com/usit-gd/nivlheim/server/service/utility"
)

type removeInactiveMachinesJob struct{}

func init() {
	RegisterJob(removeInactiveMachinesJob{})
}

func (job removeInactiveMachinesJob) HowOften() time.Duration {
	return time.Minute * 19
}

func (job removeInactiveMachinesJob) Run(db *sql.DB) {
	// Log some numbers at the end. Defer func in case of panic.
	var acount, dcount int
	defer func() {
		if acount > 0 || dcount > 0 {
			log.Printf("Archived %d machines, deleted %d machines", acount, dcount)
		}
	}()

	// How many days to wait before archiving a host
	archiveDayLimit := config.ArchiveDayLimit
	if archiveDayLimit == 0 {
		archiveDayLimit = 30 // default value is 30 days
	}

	// Find hostinfo rows that are old enough to be archived
	const query1 = "SELECT certfp FROM hostinfo" +
		" WHERE lastseen < now() - $1 * interval '1 days'"

	// Find hostinfo rows that are duplicates of the same machine.
	// This often happens if a machine is re-installed.
	// It can also happen if the certificate files are deleted somehow.
	const query2 = "SELECT certfp FROM "+
	" (SELECT certfp,rank() OVER (PARTITION BY ipaddr,os_hostname,serialno,product "+
	" ORDER BY lastseen DESC) AS pos "+
	" FROM hostinfo WHERE serialno IS NOT null AND product IS NOT null) AS ss "+
	" WHERE pos>1"
	
	// Archive the machines (delete the hostinfo entry, but keep the files)
	rows, err := db.Query(query1+" UNION "+query2, archiveDayLimit)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var certfp string
		err = rows.Scan(&certfp)
		if err != nil {
			log.Panic(err)
		}
		err = utility.RunStatementsInTransaction(db, []string{
			"UPDATE files SET current=false WHERE certfp=$1",
			"DELETE FROM hostinfo WHERE certfp=$1",
		}, certfp)
		if err != nil {
			log.Panic(err)
		} else {
			// These files should no longer show up in searches
			removeHostFromFastSearch(certfp)
			acount++
		}
	}
	if err = rows.Err(); err != nil {
		log.Panic(err)
	}
	rows.Close()

	// Delete old files and machines that I haven't heard from in a long time
	deleteDayLimit := config.DeleteDayLimit
	if deleteDayLimit == 0 {
		deleteDayLimit = 180 // default value is 180 days
	}
	rows, err = db.Query(
		"SELECT DISTINCT certfp FROM files GROUP BY certfp"+
			" HAVING max(received) < now() - $1 * interval '1 days'",
		deleteDayLimit)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var certfp string
		err = rows.Scan(&certfp)
		if err != nil {
			log.Panic(err)
		}
		err = utility.RunStatementsInTransaction(db, []string{
			"DELETE FROM hostinfo WHERE certfp=$1",
			"DELETE FROM files WHERE certfp=$1",
		}, certfp)
		if err != nil {
			log.Panic(err)
		} else {
			removeHostFromFastSearch(certfp)
			dcount++
		}
	}
	if err = rows.Err(); err != nil {
		log.Panic(err)
	}
}
