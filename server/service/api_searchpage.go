package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"html"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type apiMethodSearchPage struct {
	db      *sql.DB
	devmode bool
}

type apiSearchPageHit struct {
	FileID        int64      `json:"fileId"`
	Filename      jsonString `json:"filename"`
	IsCommand     bool       `json:"isCommand"`
	Excerpt       string     `json:"excerpt"`
	Hostname      jsonString `json:"hostname"`
	CertFP        jsonString `json:"certfp"`
	DisplayNumber int        `json:"displayNumber"`
}

type apiSearchPageResult struct {
	Query     string             `json:"query"`
	Page      int                `json:"page"`
	MaxPage   int                `json:"maxPage"`
	NumHits   int                `json:"numberOfHits"`
	Hits      []apiSearchPageHit `json:"hits"`
	Filenames []string           `json:"filenames"`
}

func (vars *apiMethodSearchPage) ServeHTTP(w http.ResponseWriter, req *http.Request, access *AccessProfile) {
	if req.Method != httpGET {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := req.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Want to distinguish between q being empty and q missing
	_, ok := req.Form["q"]
	if !ok {
		http.Error(w, "Missing parameter: q", http.StatusUnprocessableEntity)
		return
	}

	result := new(apiSearchPageResult)
	result.Query = req.FormValue("q")
	if result.Query == "" {
		// Search for an empty string yields a search result with no hits
		result.Page = 1
		result.Hits = make([]apiSearchPageHit, 0)
		returnJSON(w, req, result)
		return
	}

	result.Page, err = strconv.Atoi(req.FormValue("page"))
	if err != nil {
		result.Page = 1
	}

	var pageSize = 10
	if req.FormValue("hitsPerPage") != "" {
		if req.FormValue("hitsPerPage") == "all" {
			// Special case: You can say hitsPerPage=all to avoid pagination.
			// This is handled internally by setting pageSize to a very large number.
			const MaxUint = ^uint(0)
			const MaxInt = int(MaxUint >> 1)
			pageSize = MaxInt
		} else {
			var ps int
			if ps, err = strconv.Atoi(req.FormValue("hitsPerPage")); err == nil {
				pageSize = ps
			} else {
				http.Error(w, "Invalid hitsPerPage value", http.StatusBadRequest)
				return
			}
		}
	}

	// When the system service starts up, it can take a few seconds before the cache is loaded.
	// If we allowed search during this period, it would yield incomplete results.
	if !isReadyForSearch() {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Not ready yet, still loading data", http.StatusServiceUnavailable)
		return
	}

	filename := req.FormValue("filename")

	// Compute a list of which certificates the user has access to
	// based on current hosts in hostinfo owned by one of the groups the user has access to.
	var hitIDs []int64
	var distinctFilenames map[string]int
	statement := "SELECT certfp FROM hostinfo"
	whereAnd := "WHERE"
	if !access.HasAccessToAllGroups() {
		statement += " " + whereAnd + " ownergroup IN ("+access.GetGroupListForSQLWHERE()+")"
		whereAnd = "AND"
	}
	if config.HideUnknownHosts {
		// Hide hosts where the hostname is not determined
		statement += " " + whereAnd + " hostinfo.hostname IS NOT NULL"
	}

	// Perform the database query for the list of valid certificates
	list, err := QueryColumn(vars.db, statement)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println(err.Error())
		return
	}

	// List is a slice of interface{}, so I must convert that to a map[string]bool
	validCerts := make(map[string]bool, 100)
	for _, s := range list {
		str, ok := s.(string)
		if ok {
			validCerts[str] = true
		}
	}

	// Finally, we can perform the search
	hitIDs, distinctFilenames = searchFilesWithFilter(result.Query, filename, validCerts)

	// Put together a data structure with the results
	result.NumHits = len(hitIDs)
	result.Hits = make([]apiSearchPageHit, 0)
	result.MaxPage = int(math.Ceil(float64(result.NumHits) / float64(pageSize)))
	result.Filenames = make([]string, len(distinctFilenames))
	i := 0
	for fn := range distinctFilenames {
		result.Filenames[i] = fn
		i++
	}
	sort.Strings(result.Filenames)

	// Augment the search results with more data that's not in the cache.
	statement = "SELECT filename,is_command," +
		"COALESCE(hostname,host(hostinfo.ipaddr)),certfp,content " +
		"FROM files LEFT JOIN hostinfo USING (certfp) " +
		"WHERE fileid=$1"
	offset := (result.Page - 1) * pageSize
	for i := offset; i < offset+pageSize && i < len(hitIDs); i++ {
		fileID := hitIDs[i]
		var filename, hostname, certfp, content sql.NullString
		var isCommand sql.NullBool
		hit := apiSearchPageHit{}
		err = vars.db.QueryRow(statement, fileID).
			Scan(&filename, &isCommand, &hostname, &certfp, &content)
		if err == sql.ErrNoRows {
			log.Printf("Didn't find the file with ID %d", fileID)
			continue
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hit.FileID = fileID
		hit.Filename = jsonString(filename)
		hit.Hostname = jsonString(hostname)
		hit.CertFP = jsonString(certfp)
		hit.IsCommand = isCommand.Bool
		hit.DisplayNumber = i + 1
		hit.Excerpt = createExcerpt(fileID, content.String, result.Query)
		result.Hits = append(result.Hits, hit)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	jsonEnc := json.NewEncoder(w)
	jsonEnc.SetEscapeHTML(false)
	jsonEnc.SetIndent("", "  ")
	err = jsonEnc.Encode(result)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println(err.Error())
		return
	}
}

// Max returns the highest of the two input values
func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// Min returns the lowest of the two input values
func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func createExcerpt(fileID int64, content string, query string) string {
	var buffer bytes.Buffer
	// use a fast function to find locations where the query string matched
	for _, i := range findMatchesInFile(fileID, query, 3) {
		// include some context, try to cut off at word boundaries
		start := Max(i-30, 0)
		cutoff := strings.IndexAny(content[start:i], " \n\t")
		if cutoff != -1 {
			start = start + cutoff + 1
		}
		end := Min(i+len(query)+30, len(content))
		cutoff = strings.LastIndexAny(content[i+len(query):end], " \n\t")
		if cutoff != -1 {
			end = i + len(query) + cutoff
		}
		// html-escape and add <em>-tags
		buffer.WriteString(html.EscapeString(content[start:i]))
		buffer.WriteString("<em>")
		buffer.WriteString(html.EscapeString(content[i : i+len(query)]))
		buffer.WriteString("</em>")
		buffer.WriteString(html.EscapeString(content[i+len(query) : end]))
		buffer.WriteString("<br>")
	}
	return buffer.String()
}
