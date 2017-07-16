package main

import (
	"html/template"
	"net/http"
)

var templatePath string
var templates *template.Template

func init() {
	http.HandleFunc("/", helloworld)
	http.HandleFunc("/search", search)
}

func main() {
	//templatePath = "/var/www/nivlheim/templates"
	//cgi.Serve(nil)
	templatePath = "../templates"
	http.HandleFunc("/static/", staticfiles)
	http.ListenAndServe(":8080", nil)
}

func helloworld(w http.ResponseWriter, req *http.Request) {
	// Load html templates
	templates, err := template.ParseGlob(templatePath + "/*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fill template values
	tValues := make(map[string]interface{})
	//tValues["list"] =

	// Render template
	templates.ExecuteTemplate(w, "frontpage.html", tValues)
}

func staticfiles(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "../static/"+req.URL.Path[8:])
}
