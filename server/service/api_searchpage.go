package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"html"
	"log"
	"net/http"
	"regexp"
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
	Query   string             `json:"query"`
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
		var ps int
		if ps, err = strconv.Atoi(req.FormValue("hitsPerPage")); err == nil {
			pageSize = ps
		} else {
			http.Error(w, "Invalid hitsPerPage value", http.StatusBadRequest)
			return
		}
	}

	st := "SELECT fileid,filename,is_command,hostname,certfp,content FROM " +
		"(SELECT fileid FROM files " +
		"WHERE content ilike '%'||$1||'%' AND current " +
		"ORDER BY fileid LIMIT $2 OFFSET $3) as foo " +
		"LEFT JOIN files USING (fileid) " +
		"LEFT JOIN hostinfo USING (certfp) " +
		"WHERE hostname IS NOT NULL"

	/*if vars.devmode {
		var rows *sql.Rows
		rows, err = vars.db.Query("EXPLAIN "+st, result.Query, pageSize,
			(result.Page-1)*pageSize)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for rows.Next() {
			var s string
			if err = rows.Scan(&s); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			log.Println(s)
		}
		if err = rows.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}*/

	rows, err := vars.db.Query(st, result.Query, pageSize,
		(result.Page-1)*pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result.Hits = make([]apiSearchPageHit, 0)
	for rowNumber := 1; rows.Next(); rowNumber++ {
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
		hit.DisplayNumber = rowNumber
		hit.Excerpt = createExcerpt(content.String, result.Query)
		result.Hits = append(result.Hits, hit)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result.MaxPage = result.Page + 1
	if result.MaxPage < 10 {
		result.MaxPage = 10
	}
	//result.MaxPage = int(math.Ceil(float64(result.NumHits) / float64(pageSize)))

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

func createExcerpt(content string, query string) string {
	re := regexp.MustCompile(`(?i)\w{0,10}?.{0,20}` + query + `.{0,20}\w{0,10}?`)
	escQ := html.EscapeString(strings.ToLower(query))
	var buffer bytes.Buffer
	for _, found := range re.FindAllString(content, 3) {
		// html-escape, then add <em>-tags
		found := html.EscapeString(found)
		if i := strings.Index(strings.ToLower(found), escQ); i > -1 {
			buffer.WriteString(found[0:i])
			buffer.WriteString("<em>")
			buffer.WriteString(found[i : i+len(escQ)])
			buffer.WriteString("</em>")
			buffer.WriteString(found[i+len(escQ):])
		} else {
			buffer.WriteString(found)
		}
		buffer.WriteString("<br>")
	}
	return buffer.String()
}
