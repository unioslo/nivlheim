package main

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
)

// AccessProfile holds information about what data and
// which admin functions the owner is allowed access to.
type AccessProfile struct {
	certs   map[string]bool
	isAdmin bool
}

func (ap *AccessProfile) HasAccessTo(certfp string) bool {
	return ap.certs[certfp]
}

func (ap *AccessProfile) IsAdmin() bool {
	return ap.isAdmin
}

func GenerateAccessProfileForUser(username string) (*AccessProfile, error) {
	//TODO: Don't hardcode the url
	resp, err := http.Get(
		//"http://localhost/cgi-bin/brukerTilSiteAdmin.pl?username=" + url.QueryEscape(username)
		"http://localhost:8080/access.json")
	if err != nil {
		return nil, err
	}
	jsonbytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var array []string
	err = json.Unmarshal(jsonbytes, &array)
	if err != nil {
		return nil, err
	}
	ap := new(AccessProfile)
	ap.certs = make(map[string]bool)
	for _, s := range array {
		ap.certs[s] = true
	}
	ap.isAdmin = false
	//TODO might use this later:
	// ap.created = time.Now()
	// ap.key = username
	return ap, nil
}

func getRealRemoteAddr(req *http.Request) net.IP {
	// Only works if there's exactly one proxy (no more, no less).
	// If there's no proxy, the client could spoof the XFF header.
	ff, ok := req.Header["X-Forwarded-For"]
	if ok {
		return net.ParseIP(ff[len(ff)-1])
	}
	return net.ParseIP(strings.Split(req.RemoteAddr, ":")[0])
}
