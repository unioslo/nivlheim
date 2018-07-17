package main

import (
	"database/sql"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

var fsMutex sync.RWMutex
var fsContent map[int64]string
var fsID map[string]int64
var fsKey map[int64]string
var fsReady bool

func init() {
	fsContent = make(map[int64]string)
	fsID = make(map[string]int64)
	fsKey = make(map[int64]string)
}

func isReadyForSearch() bool {
	return fsReady
}

func loadContentForFastSearch(db *sql.DB) {
	fsMutex.Lock()
	defer fsMutex.Unlock()
	log.Printf("Starting to load file content for fast search")
	rows, err := db.Query("SELECT fileid,filename,certfp,content FROM files " +
		"WHERE current")
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var fileID int64
		var filename, certfp, content sql.NullString
		err = rows.Scan(&fileID, &filename, &certfp, &content)
		if err != nil {
			log.Panic(err)
		}
		if !certfp.Valid || !filename.Valid || !content.Valid {
			continue
		}
		fsContent[fileID] = strings.ToLower(content.String)
		key := certfp.String + ":" + filename.String
		fsID[key] = fileID
		fsKey[fileID] = key
	}
	log.Printf("Finished loading file content for fast search")
	fsReady = true
}

func addFileToFastSearch(fileID int64, certfp string, filename string, content string) {
	fsMutex.Lock()
	defer fsMutex.Unlock()
	fsContent[fileID] = content
	key := certfp + ":" + filename
	fsID[key] = fileID
	fsKey[fileID] = key
}

func removeFileFromFastSearch(fileID int64) {
	fsMutex.Lock()
	defer fsMutex.Unlock()
	delete(fsContent, fileID)
	key, ok := fsKey[fileID]
	if ok {
		delete(fsID, key)
	}
	delete(fsKey, fileID)
}

func compareSearchCacheToDB(db *sql.DB) {
	// read a list of "current" file IDs from the database
	source := make(map[int64]bool, 10000)
	rows, err := db.Query("SELECT fileid FROM files WHERE current")
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var fileID int64
		err = rows.Scan(&fileID)
		if err != nil {
			log.Panic(err)
		}
		source[fileID] = true
	}
	if rows.Err() != nil {
		log.Panic(rows.Err())
	}
	// find entries in the cache that should have been removed
	fsMutex.RLock()
	defer fsMutex.RUnlock()
	var obsoleteCount int
	for fileID := range fsKey {
		if _, ok := source[fileID]; !ok {
			obsoleteCount++
			// fix it by removing the obsolete entry
			delete(fsContent, fileID)
			key, ok := fsKey[fileID]
			if ok {
				delete(fsID, key)
			}
			delete(fsKey, fileID)
		}
	}
	if obsoleteCount > 0 {
		log.Printf("The search cache had %d files that were obsolete", obsoleteCount)
	}
	// find missing entries
	var missingCount int
	for fileID := range source {
		if _, ok := fsKey[fileID]; !ok {
			missingCount++
			// fix it by loading the missing entry
			var filename, certfp, content sql.NullString
			err := db.QueryRow("SELECT filename,certfp,content FROM files "+
				"WHERE fileid=$1 AND current", fileID).
				Scan(&filename, &certfp, &content)
			if err == sql.ErrNoRows {
				continue
			} else if err != nil {
				log.Panic(err)
			}
			if !certfp.Valid || !filename.Valid || !content.Valid {
				continue
			}
			fsContent[fileID] = strings.ToLower(content.String)
			key := certfp.String + ":" + filename.String
			fsID[key] = fileID
			fsKey[fileID] = key
		}
	}
	if missingCount > 0 {
		log.Printf("The search cache was missing %d files", missingCount)
	}
	if missingCount == 0 && obsoleteCount == 0 {
		log.Printf("The search cache was up-to-date.")
	}
}

type hitList []int64

func (a hitList) Len() int           { return len(a) }
func (a hitList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a hitList) Less(i, j int) bool { return a[i] > a[j] } // reverse sort

func searchFiles(searchString string) []int64 {
	fsMutex.RLock()
	defer fsMutex.RUnlock()
	searchString = strings.ToLower(searchString)
	hits := make(hitList, 0, 0)
	for id, content := range fsContent {
		if strings.Contains(content, searchString) {
			hits = append(hits, id)
		}
	}
	// The result list must be in the same order every time for pagination to work.
	// The hits are reverse sorted so the newest files will show first.
	sort.Sort(hits)
	return hits
}

func findMatchesInFile(fileID int64, query string, maxMatches int) []int {
	fsMutex.RLock()
	content, ok := fsContent[fileID]
	fsMutex.RUnlock()
	if !ok {
		return nil
	}
	result := make([]int, 0, maxMatches)
	query = strings.ToLower(query)
	offset := 0
	for n := 0; n < maxMatches; n++ {
		i := strings.Index(content, query)
		if i == -1 {
			break
		}
		result = append(result, i+offset)
		content = content[i+len(query):]
		offset = offset + i + len(query)
	}
	return result
}

// Job
type compareSearchCacheJob struct{}

func init() {
	RegisterJob(compareSearchCacheJob{})
}

func (job compareSearchCacheJob) HowOften() time.Duration {
	return time.Minute * 15
}

func (job compareSearchCacheJob) Run(db *sql.DB) {
	compareSearchCacheToDB(db)
}
