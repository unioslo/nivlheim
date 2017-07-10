package web

import (
	"fmt"
	"net/http"
	"net/http/cgi"
)

func init() {
	http.HandleFunc("/", helloworld)
}

func main() {
	cgi.Serve(nil)
}

func helloworld(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "Hei verden!")
}
