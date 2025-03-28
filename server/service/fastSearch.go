package main

import (
	"database/sql"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var fsMutex sync.RWMutex
var fsContent map[int64]string
var fsID map[string]int64  // maps a key string to file id. The key is <certfp>:<filename>
var fsKey map[int64]string // the reverse of fsID
var fsReady uint32

func init() {
	fsContent = make(map[int64]string)
	fsID = make(map[string]int64)
	fsKey = make(map[int64]string)
}

func isReadyForSearch() bool {
	i := atomic.LoadUint32(&fsReady)
	return i == 1
}

func loadContentForFastSearch(db *sql.DB) {
	log.Printf("Starting to load file content for fast search")
	rows, err := db.Query("SELECT fileid,filename,certfp,content FROM files " +
		"WHERE current AND certfp IN (SELECT certfp FROM hostinfo)")
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
		addFileToFastSearch(fileID, certfp.String, filename.String, content.String)
	}
	log.Printf("Finished loading file content for fast search")
	atomic.StoreUint32(&fsReady, 1)
	// trigger the job
	triggerJob(compareSearchCacheJob{})
}

// Adds (or replaces) a file in the in-memory search cache
func addFileToFastSearch(fileID int64, certfp string, filename string, content string) {
	fsMutex.Lock()
	defer fsMutex.Unlock()
	key := certfp + ":" + filename
	// If a previous version of the file is in the cache, it should be removed
	oldID, ok := fsID[key]
	if ok {
		delete(fsKey, oldID)
		delete(fsContent, oldID)
	}
	fsContent[fileID] = strings.ToLower(content)
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

func removeHostFromFastSearch(certFingerprint string) {
	fsMutex.Lock()
	defer fsMutex.Unlock()
	for key, fileID := range fsID {
		ar := strings.SplitN(key, ":", 2)
		if ar[0] == certFingerprint {
			delete(fsContent, fileID)
			delete(fsKey, fileID)
			delete(fsID, key)
		}
	}
}

func replaceCertificateInCache(oldCertFingerprint, newCertFingerprint string) {
	fsMutex.Lock()
	defer fsMutex.Unlock()
	for key, fileID := range fsID {
		ar := strings.SplitN(key, ":", 2)
		if ar[0] == oldCertFingerprint {
			newKey := newCertFingerprint + ":" + ar[1]
			fsID[newKey] = fileID
			fsKey[fileID] = newKey
			delete(fsID, key)
		}
	}
}

func numberOfFilesInFastSearch() int {
	// Don't want to return a count if the cache isn't fully loaded yet, it would be misleading
	if !isReadyForSearch() {
		return -1
	}
	fsMutex.RLock()
	defer fsMutex.RUnlock()
	return len(fsKey)
}

func compareSearchCacheToDB(db *sql.DB) {
	// No point in doing this until the cache has been initially populated
	if !isReadyForSearch() {
		return
	}

	// Files are added and removed all the time.
	// Even when locking the cache by mutex, there's a chance that something
	// updates the database simultaneously and the cache won't be 100% in sync
	// at the time of testing.
	// A way to get around this is to run two passes with a few seconds in between,
	// and only use the results that are persistent.
	const passes = 2
	obsolete := make([]map[int64]bool, passes)
	missing := make([]map[int64]bool, passes)
	for pass := 0; pass < passes; pass++ {

		// Read a list of the IDs of "current" and parsed files from the database
		source := make(map[int64]bool, 10000)
		rows, err := db.Query("SELECT fileid FROM files WHERE current AND certfp IN (SELECT certfp FROM hostinfo)")
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

		// Allocate maps
		obsolete[pass] = make(map[int64]bool)
		missing[pass] = make(map[int64]bool)

		// Find entries in the cache that should have been removed
		fsMutex.RLock()
		for fileID := range fsKey {
			if _, ok := source[fileID]; !ok {
				obsolete[pass][fileID] = true
			}
		}

		// Find files that are missing from the cache
		for fileID := range source {
			_, ok := fsKey[fileID]
			if !ok {
				missing[pass][fileID] = true
			}
		}
		fsMutex.RUnlock()

		// Sleep between passes
		if pass < passes-1 {
			time.Sleep(time.Second * 5)
		}
	}

	// Remove the obsolete entries
	rem := 0
	for fileID, b := range obsolete[0] {
		if b && obsolete[1][fileID] {
			removeFileFromFastSearch(fileID)
			rem++
		}
	}
	if rem > 0 {
		log.Printf("The search cache had %d files that were obsolete", rem)
	}

	// Load the missing files
	mis := 0
	for fileID, b := range missing[0] {
		if b && missing[1][fileID] {
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
			addFileToFastSearch(fileID, certfp.String, filename.String, content.String)
			mis++
		}
	}
	if mis > 0 {
		log.Printf("The search cache was missing %d files", mis)
	}
}

type hitList []int64

func (a hitList) Len() int           { return len(a) }
func (a hitList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a hitList) Less(i, j int) bool { return a[i] > a[j] } // reverse sort

func searchFiles(searchString string, filename string) ([]int64, map[string]int) {
	fsMutex.RLock()
	searchString = strings.ToLower(searchString)
	hits := make(hitList, 0)
	distinctFilenames := make(map[string]int, 0)
	for id, content := range fsContent {
		if strings.Contains(content, searchString) {
			key := fsKey[id]
			ar := strings.SplitN(key, ":", 2)
			if filename != "" {
				if filename != ar[1] {
					continue
				}
			}
			hits = append(hits, id)
			distinctFilenames[ar[1]]++
		}
	}
	fsMutex.RUnlock()
	// The result list must be in the same order every time for pagination to work.
	// The hits are reverse sorted so the newest files will show first.
	sort.Sort(hits)
	return hits, distinctFilenames
}

func searchFilesWithFilter(searchString string, filename string, validCerts map[string]bool) ([]int64, map[string]int) {
	fsMutex.RLock()
	searchString = strings.ToLower(searchString)
	hits := make(hitList, 0)
	distinctFilenames := make(map[string]int, 0)
	for key, id := range fsID {
		// extract certfp and filename from key
		ar := strings.SplitN(key, ":", 2)
		if filename != "" && filename != ar[1] {
			continue
		}
		certfp := ar[0]
		if validCerts[certfp] {
			content := fsContent[id]
			if strings.Contains(content, searchString) {
				hits = append(hits, id)
				distinctFilenames[ar[1]]++
			}
		}
	}
	fsMutex.RUnlock()
	// The result list must be in the same order every time for pagination to work.
	// The hits are reverse sorted so the newest files will show first.
	sort.Sort(hits)
	return hits, distinctFilenames
}

func searchForHosts(searchString string, filename string) map[string]bool {
	fsMutex.RLock()
	defer fsMutex.RUnlock()
	searchString = strings.ToLower(searchString)
	resultMap := make(map[string]bool, 0)
	for key, id := range fsID {
		// extract certfp and filename from key
		ar := strings.SplitN(key, ":", 2)
		if filename != "" && filename != ar[1] {
			continue
		}
		// match strings
		if strings.Contains(fsContent[id], searchString) {
			resultMap[ar[0]] = true
		}
	}
	return resultMap
}

func findMatchesInFile(fileID int64, query string, maxMatches int) []int {
	fsMutex.RLock()
	content, ok := fsContent[fileID]
	fsMutex.RUnlock()
	if !ok {
		return nil
	}
	result := make([]int, 0, Min(maxMatches, 10))
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

// getCertAndFilenameFromFileID returns 2 strings: certificate fingerprint and filename
func getCertAndFilenameFromFileID(fileID int64) (string, string) {
	fsMutex.RLock()
	defer fsMutex.RUnlock()
	ar := strings.Split(fsKey[fileID], ":")
	if ar == nil || len(ar) < 2 {
		// Should not happen, but if it does, do this to avoid a crash.
		return "", ""
	}
	return ar[0], ar[1]
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
