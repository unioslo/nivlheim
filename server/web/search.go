package main

import (
	"database/sql"
	"html/template"
	"net/http"
	"strings"

	_ "github.com/lib/pq"
)

type Hit struct {
	fileid   int
	filename string
	excerpt  string
	hostname string
	number   int
}

func search(w http.ResponseWriter, req *http.Request) {
	db, err := sql.Open("postgres", "host=potetgull.mooo.com "+
		"dbname=apache sslmode=disable user=apache")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	hits := make([]Hit, 0, 0)
	query := req.FormValue("q")
	rows, err := db.Query("SELECT fileid,filename,content,certcn "+
		"FROM files WHERE content LIKE $1 ORDER BY received DESC "+
		"LIMIT 10", "%"+query+"%")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		defer rows.Close()
		for no := 1; rows.Next(); no++ {
			var hit Hit
			var content string
			rows.Scan(&hit.fileid, &hit.filename, &content, &hit.hostname)
			hit.number = no
			i := strings.Index(content, query)
			start := i - 20
			if start < 0 {
				start = 0
			}
			end := i + 20
			if end >= len(content) {
				end = len(content) - 1
			}
			hit.excerpt = content[start:end]
			i = strings.Index(hit.excerpt, query)
			i2 := i + len(query)
			hit.excerpt = hit.excerpt[0:i2] + "</b>" + hit.excerpt[i2:]
			hit.excerpt = hit.excerpt[0:i] + "<b>" + hit.excerpt[i:]
			hits = append(hits, hit)
		}
	}

	// Load templates
	templates, err := template.ParseGlob(templatePath + "/*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fill template values
	tValues := make(map[string]interface{})
	tValues["q"] = req.FormValue("q")
	tValues["hits"] = hits

	// Render template
	templates.ExecuteTemplate(w, "searchpage.html", tValues)
}
