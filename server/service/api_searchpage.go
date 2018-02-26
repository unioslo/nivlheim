package main

import (
	"database/sql"
	"encoding/json"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type apiMethodSearchPage struct {
	db *sql.DB
}

type apiSearchPageHit struct {
	FileID        int        `json:"fileId"`
	Filename      jsonString `json:"filename"`
	IsCommand     bool       `json:"isCommand"`
	Excerpt       string     `json:"excerpt"`
	Hostname      jsonString `json:"hostname"`
	CertFP        jsonString `json:"certfp"`
	DisplayNumber int        `json:"displayNumber"`
}

type apiSearchPageResult struct {
	Query   string             `json:"query"`
	NumHits int                `json:"numberOfHits"`
	Page    int                `json:"page"`
	Hits    []apiSearchPageHit `json:"hits"`
}

func (vars *apiMethodSearchPage) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	result := new(apiSearchPageResult)
	err := req.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, ok := req.Form["q"]
	if !ok {
		http.Error(w, "Missing parameter: q", http.StatusUnprocessableEntity)
		return
	}
	result.Query = req.FormValue("q")
	if result.Query == "" {
		result.Page = 1
		result.NumHits = 0
		result.Hits = make([]apiSearchPageHit, 0)
		returnJSON(w, req, result)
		return
	}

	result.Page, err = strconv.Atoi(req.FormValue("page"))
	if err != nil {
		result.Page = 1
	}

	row, err := vars.db.Query(
		"SELECT count(*) FROM (SELECT content,row_number() OVER "+
			"(PARTITION BY certfp,filename ORDER BY mtime DESC) "+
			"FROM files) AS foo "+
			"WHERE row_number=1 AND content ILIKE '%'||$1||'%'",
		result.Query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer row.Close()
	if row.Next() {
		err = row.Scan(&result.NumHits)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	var pageSize = 10
	if req.FormValue("hitsPerPage") != "" {
		var ps int
		if ps, err = strconv.Atoi(req.FormValue("hitsPerPage")); err == nil {
			pageSize = ps
		} else {
			http.Error(w, "Invalid hitsPerPage value", http.StatusBadRequest)
			return
		}
	}

	st := "SELECT fileid,filename,is_command,hostname,certfp,content " +
		"FROM (SELECT fileid,filename,is_command,certfp,content," +
		"row_number() OVER (PARTITION BY certfp,filename ORDER BY mtime DESC) " +
		"FROM files) AS foo LEFT JOIN hostinfo USING (certfp) " +
		"WHERE row_number=1 AND content ILIKE '%'||$1||'%' " +
		"ORDER BY hostname LIMIT $2 OFFSET $3"

	rows, err := vars.db.Query(st, result.Query, pageSize,
		(result.Page-1)*pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	lq := strings.ToLower(html.EscapeString(result.Query))
	exSize, err := strconv.Atoi(req.FormValue("excerpt"))
	if err != nil {
		exSize = 50
	}

	result.Hits = make([]apiSearchPageHit, 0)
	for displayNumber := 1; rows.Next(); displayNumber++ {
		var filename, hostname, certfp, content sql.NullString
		var isCommand sql.NullBool
		hit := apiSearchPageHit{}
		err = rows.Scan(&hit.FileID, &filename, &isCommand, &hostname, &certfp,
			&content)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hit.Filename = jsonString(filename)
		hit.Hostname = jsonString(hostname)
		hit.CertFP = jsonString(certfp)
		hit.IsCommand = isCommand.Bool
		hit.DisplayNumber = displayNumber
		// must html-escape the content excerpt
		ec := html.EscapeString(content.String)
		i := strings.Index(strings.ToLower(ec), lq)
		start := i - exSize/2
		if start < 0 {
			start = 0
		}
		end := i + exSize/2
		if end > len(ec) {
			end = len(ec)
		}
		ex := ec[start:end]
		i = strings.Index(strings.ToLower(ex), lq)
		i2 := i + len(lq)
		ex = ex[0:i2] + "</em>" + ex[i2:]
		ex = ex[0:i] + "<em>" + ex[i:]
		if len(ex) > 0 && ex[len(ex)-1] != ' ' {
			ex += "&hellip;"
		}
		hit.Excerpt = ex
		result.Hits = append(result.Hits, hit)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
