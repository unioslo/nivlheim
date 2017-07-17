package main

import (
	"database/sql"
	"html/template"
	"net/http"
	"strings"

	"github.com/lib/pq"
)

type Hit struct {
	Fileid   int
	Filename string
	Excerpt  template.HTML
	Hostname string
	Number   int
}

func search(w http.ResponseWriter, req *http.Request) {
	db, err := sql.Open("postgres", dbConnectionString)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	hits := make([]Hit, 0, 0)
	query := strings.TrimSpace(req.FormValue("q"))
	lowerq := strings.ToLower(query)
	var count int

	if query != "" {
		// Number of hits
		err = db.QueryRow("SELECT count(distinct(filename,certcn)) FROM files "+
			"WHERE lower(content) LIKE '%' || $1 || '%' ", lowerq).Scan(&count)
		if err != nil && err != sql.ErrNoRows {
			http.Error(w, "2: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Hit list
		rows, err := db.Query("SELECT filename,certcn,max(received) "+
			"FROM files "+
			"WHERE lower(content) LIKE '%' || $1 || '%' "+
			"GROUP BY filename,certcn "+
			"ORDER BY max(received) DESC", lowerq)
		if err != nil {
			http.Error(w, "1: "+err.Error(), http.StatusInternalServerError)
			return
		} else {
			defer rows.Close()
			for no := 1; rows.Next(); no++ {
				var hit Hit
				var received pq.NullTime
				err = rows.Scan(&hit.Filename, &hit.Hostname, &received)
				if err != nil {
					http.Error(w, "3: "+err.Error(), http.StatusInternalServerError)
					return
				}
				hit.Number = no
				var content string
				err = db.QueryRow("SELECT fileid, content FROM files "+
					"WHERE filename=$1 AND certcn=$2 "+
					"ORDER BY received DESC LIMIT 1",
					hit.Filename, hit.Hostname).
					Scan(&hit.Fileid, &content)
				switch {
				case err == sql.ErrNoRows:
					continue
				case err != nil:
					http.Error(w, "2: "+err.Error(), http.StatusInternalServerError)
					return
				}
				i := strings.Index(strings.ToLower(content), lowerq)
				start := i - 20
				if start < 0 {
					start = 0
				}
				end := i + len(query) + 20
				if end >= len(content) {
					end = len(content) - 1
				}
				ex := content[start:end]
				lowerex := strings.ToLower(ex)
				i = strings.Index(lowerex, lowerq)
				if i > -1 {
					i2 := i + len(query)
					ex = ex[0:i2] + "</em>" + ex[i2:]
					ex = ex[0:i] + "<em>" + ex[i:]
				}
				hit.Excerpt = template.HTML(ex)
				hits = append(hits, hit)
			}
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
	tValues["count"] = count

	// Render template
	err = templates.ExecuteTemplate(w, "searchpage.html", tValues)
	if err != nil {
		s := "<hr><p>Error:<br><pre>" + err.Error() + "</pre></p>"
		w.Write([]byte(s))
	}
}
