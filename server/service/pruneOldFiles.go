package main

import (
	"database/sql"
	"log"
	"time"

	"github.com/lib/pq"
)

type pruneOldFilesJob struct{}

var pruneTimeTable []time.Duration

func init() {
	pruneTimeTable = generateTimeTable()
	RegisterJob(pruneOldFilesJob{})
}

func (p pruneOldFilesJob) HowOften() time.Duration {
	return time.Hour
}

func (p pruneOldFilesJob) Run(db *sql.DB) {
	// Find all machines
	machineList := make([]string, 0, 100)
	rows, err := db.Query("SELECT DISTINCT certfp FROM files")
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var certfp sql.NullString
		err = rows.Scan(&certfp)
		if err != nil {
			log.Panic(err)
		}
		if certfp.Valid {
			machineList = append(machineList, certfp.String)
		}
	}
	if err = rows.Err(); err != nil {
		log.Panic(err)
	}
	rows.Close()

	// For every machine
	for _, certfp := range machineList {
		// Finn all unique filenames on the machine
		rows, err = db.Query("SELECT DISTINCT filename FROM files "+
			"WHERE certfp=$1", certfp)
		if err != nil {
			log.Panic(err)
		}
		filenames := make([]string, 0)
		for rows.Next() {
			var filename sql.NullString
			err = rows.Scan(&filename)
			if err != nil {
				log.Panic(err)
			}
			if filename.Valid {
				filenames = append(filenames, filename.String)
			}
		}
		if err = rows.Err(); err != nil {
			log.Panic(err)
		}
		// Can't wait with rows.Close() until the function ends;
		// If many machines, it would cause too many open connections.
		rows.Close()

		// For every file
		for _, filename := range filenames {
			timeMap := make(map[int]time.Time)
			// Find all versions of that file
			rows, err = db.Query("SELECT fileid,mtime FROM files "+
				"WHERE certfp=$1 AND filename=$2", certfp, filename)
			if err != nil {
				log.Panic(err)
			}
			for rows.Next() {
				var fileID sql.NullInt64
				var mtime pq.NullTime
				err = rows.Scan(&fileID, &mtime)
				if err != nil {
					log.Panic(err)
				}
				if fileID.Valid && mtime.Valid {
					timeMap[int(fileID.Int64)] = mtime.Time
				}
			}
			if err = rows.Err(); err != nil {
				log.Panic(err)
			}
			rows.Close()

			// Find what to delete
			var count int
			for _, deleteID := range whatToDelete(&timeMap) {
				_, err = db.Exec("DELETE FROM files WHERE fileid=$1", deleteID)
				if err != nil {
					log.Panic(err)
				}
				count++
			}
			if count > 0 {
				//log.Printf("Pruned %d files from the database.\n", count)
			}
		}
	}
}

func whatToDelete(m *map[int]time.Time) []int {
	type record struct {
		fileID int
		mtime  time.Time
	}

	slots := make([]*record, len(pruneTimeTable))
	for id, timestamp := range *m {
		// find the right slot for this record
		age := time.Since(timestamp)
		slot := 0
		for slot < len(pruneTimeTable) && age > pruneTimeTable[slot] {
			slot++
		}
		// if it is older than the record currently occupying the slot,
		// it can have it.
		if slots[slot] == nil || timestamp.Before(slots[slot].mtime) {
			slots[slot] = &record{
				fileID: id,
				mtime:  timestamp,
			}
		}
	}

	// what to keep
	keep := make(map[int]bool)
	for _, r := range slots {
		if r != nil {
			keep[r.fileID] = true
		}
	}

	// keep the newest(latest) and the oldest(earliest) version forever
	var oldest, newest record
	now := time.Now()
	for i, t := range *m {
		if oldest.fileID == 0 || oldest.mtime.After(t) {
			oldest.fileID = i
			oldest.mtime = t
		}
		if newest.fileID == 0 || newest.mtime.Before(t) {
			newest.fileID = i
			newest.mtime = t
		}
		// keep every version from the last 24 hours
		if now.Sub(t) < time.Duration(24)*time.Hour {
			keep[i] = true
		}
	}
	keep[oldest.fileID] = true
	keep[newest.fileID] = true

	// Ok, now we have a list of what to keep.
	// Let's find what to delete.
	del := make([]int, 0, len(*m)-len(keep))
	for i := range *m {
		if !keep[i] {
			del = append(del, i)
		}
	}
	return del
}

// generateTimeTable returns a slice of time.Time.
// The intention is that each entry becomes a slot
// where one record can be kept, as long as it is
// younger than the age given by the entry.
// The entries are given in ascending order.
func generateTimeTable() []time.Duration {
	result := make([]time.Duration, 0)
	// keep one version per day for the last 30 days
	t := time.Duration(0)
	for days := 1; days <= 30; days++ {
		t += time.Hour * 24
		result = append(result, t)
	}
	// keep one version per week for 52 weeks after that
	for weeks := 1; weeks <= 52; weeks++ {
		t += time.Hour * 24 * 7
		result = append(result, t)
	}
	// keep one version per month (30 days) for 10 years after that
	for months := 1; months <= 12*10; months++ {
		t += time.Hour * 24 * 30
		result = append(result, t)
	}
	return result
}
