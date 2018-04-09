package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
)

type apiMethodSearchPage struct {
	db *sql.DB
}

type apiSearchPageHit struct {
	FileID        int64      `json:"fileId"`
	Filename      jsonString `json:"filename"`
	IsCommand     bool       `json:"isCommand"`
	Excerpt       jsonString `json:"excerpt"`
	Hostname      jsonString `json:"hostname"`
	CertFP        jsonString `json:"certfp"`
	DisplayNumber int        `json:"displayNumber"`
}

type apiSearchPageResult struct {
	Query   string             `json:"query"`
	NumHits int                `json:"numberOfHits"`
	Page    int                `json:"page"`
	MaxPage int                `json:"maxPage"`
	Hits    []apiSearchPageHit `json:"hits"`
}

func (vars *apiMethodSearchPage) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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
		"SELECT count(distinct(certfp,filename)) FROM files "+
			"WHERE tsvec @@ plainto_tsquery('english',$1)",
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

	result.MaxPage = int(math.Ceil(float64(result.NumHits) / float64(pageSize)))

	st := "SELECT fileid,filename,is_command,hostname,certfp,content," +
		"ts_headline(content,plainto_tsquery('english',$1)) AS excerpt FROM " +
		" (SELECT fileid,filename,is_command,certfp,content FROM " +
		"   (SELECT fileid,filename,is_command,certfp,content,tsvec,row_number()" +
		"    OVER (PARTITION BY certfp,filename ORDER BY mtime DESC)" +
		"    FROM files WHERE tsvec @@ plainto_tsquery('english',$1)" +
		"   ) AS foo1 " +
		"   WHERE row_number=1 " +
		"   ORDER BY ts_rank(tsvec, plainto_tsquery('english',$1)) DESC " +
		"   LIMIT $2 OFFSET $3" +
		" ) AS foo2 " +
		" LEFT JOIN hostinfo USING (certfp)"

	rows, err := vars.db.Query(st, result.Query, pageSize,
		(result.Page-1)*pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result.Hits = make([]apiSearchPageHit, 0)
	for rowNumber := 1; rows.Next(); rowNumber++ {
		var filename, hostname, certfp, content, excerpt sql.NullString
		var isCommand sql.NullBool
		hit := apiSearchPageHit{}
		err = rows.Scan(&hit.FileID, &filename, &isCommand, &hostname, &certfp,
			&content, &excerpt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hit.Filename = jsonString(filename)
		hit.Hostname = jsonString(hostname)
		hit.CertFP = jsonString(certfp)
		hit.IsCommand = isCommand.Bool
		hit.DisplayNumber = rowNumber
		hit.Excerpt = jsonString(excerpt)
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
