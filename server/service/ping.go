package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
)

type apiMethodSecurePing struct {
	db *sql.DB
}

func apiPing(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("pong"))
}

func (vars *apiMethodSecurePing) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var err error
	var left int64

	// If the client cert will expire soon, politely ask it to renew
	left, err = strconv.ParseInt(req.Header.Get("Cert-Client-V-Remain"), 10, 64)
	if err != nil {
		http.Error(w, "Unable to get certificate time remaining", http.StatusInternalServerError)
		return
	}
	if left < 30 {
		http.Error(w, "Your certificate is about to expire, please renew it.", http.StatusBadRequest)
		return
	}

	// If the client cert was signed by a different CA than the one that's currently active,
	// politely ask it to renew
	cIssuer := req.Header.Get("Cert-Client-I-DN")
	ca, err := os.ReadFile("/var/www/nivlheim/CA/nivlheimca.crt")
	if err != nil {
		log.Println(err.Error())
		http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
		return
	}

	cacert := getCert(ca)
	if cacert == nil {
		http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
		return
	}
	caIssuer := cacert.Subject.ToRDNSequence()

	if cIssuer != caIssuer.String() {
		http.Error(w, "The server has a new CA certificate, please renew your certificate.", http.StatusBadRequest)
		return
	}

	// Compute the client cert fingerprint
	pemContent := req.Header.Get("Cert-Client-Cert")

	// pem.Decode needs the header and footer to be on their own lines
	match := regexp.MustCompile("(?s)(-{5}BEGIN CERTIFICATE-{5})(.*)(-{5}END CERTIFICATE-{5})")
	pemContent2 := match.ReplaceAll([]byte(pemContent), []byte("$1\n$2\n$3"))

	cert := getCert(pemContent2)
	if cert == nil {
		http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
		return
	}

	fingerprint := getCertFPString(cert)

	// Check revoked status
	var revoked bool
	err = vars.db.QueryRow("SELECT revoked FROM certificates WHERE fingerprint=$1", fingerprint).Scan(&revoked)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("Could not find certificate in database when checking revocation status. Fingerprint: %s", fingerprint)
			http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
			return
		} else {
			log.Println(err.Error())
			http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
			return
		}
	}
	if revoked {
		http.Error(w, "Your certificate has been revoked.", http.StatusForbidden)
		return
	}

	// Check if the machine has been renamed (compare commonname with the current hostname)
	dn := req.Header.Get("Cert-Client-S-DN")
	match = regexp.MustCompile("CN=(.*?),.*$")
	cn := match.ReplaceAll([]byte(dn), []byte("$1"))
	var hostname sql.NullString

	err = vars.db.QueryRow("SELECT hostname FROM hostinfo WHERE certfp=$1", fingerprint).Scan(&hostname)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("Could not find certificate in database when checking hostname. Fingerprint: %s", fingerprint)
		} else {
			log.Println(err.Error())
			http.Error(w, "The server encountered an error. Please try again later.", http.StatusInternalServerError)
			return
		}
	}
	if len(hostname.String) > 0 && hostname.String != string(cn) {
		http.Error(w, "Please renew your certificate.", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("pong"))
}
