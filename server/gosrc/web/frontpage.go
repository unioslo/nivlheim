package web

import (
	"html/template"
	"net/http"
	"net/http/cgi"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
)

func init() {
	http.HandleFunc("/", helloworld)
}

func main() {
	cgi.Serve(nil)
}

func helloworld(w http.ResponseWriter, req *http.Request) {
	ctx := appengine.NewContext(req)

	// Load html templates
	var err error
	templates, err := template.ParseGlob("/var/www/nivlheim/templates/*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Errorf(ctx, err.Error())
		return
	}
	log.Infof(ctx, "%s", templates.DefinedTemplates())

	// Fill template values
	tValues := make(map[string]interface{})
	//tValues["list"] =

	// Render template
	templates.ExecuteTemplate(w, "frontpage.html", tValues)
}
