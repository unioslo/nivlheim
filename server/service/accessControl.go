package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
)

// AccessProfile holds information about which hosts the user is allowed access to,
// and whether the user has admin rights.
type AccessProfile struct {
	certs   map[string]bool
	isAdmin bool
}

func (ap *AccessProfile) HasAccessTo(certfp string) bool {
	return ap.isAdmin || ap.certs[certfp]
}

func (ap *AccessProfile) IsAdmin() bool {
	return ap.isAdmin
}

func GenerateAccessProfileForUser(userID string) (*AccessProfile, error) {
	if authorizationPluginURL == "" {
		// If no authorization plugin is defined,
		// then by default, let everyone have admin rights.
		ap := new(AccessProfile)
		ap.isAdmin = true
		ap.certs = make(map[string]bool)
		return ap, nil
	}
	resp, err := http.Get(authorizationPluginURL + url.QueryEscape(userID))
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

// ------------------------- http helpers -----------

type httpHandlerWithAccessProfile interface {
	ServeHTTP(http.ResponseWriter, *http.Request, *AccessProfile)
}

// wrapRequireAdmin adds a layer that requires that there is
// an interactive user session and that the user has admin rights.
func wrapRequireAdmin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// If authentication is not enabled in config, let the request through
		if !authRequired {
			h.ServeHTTP(w, req)
			return
		}
		session := getSessionFromRequest(req)
		if session == nil {
			// The user isn't logged in
			http.Error(w, "Not logged in", http.StatusUnauthorized)
			return
		}
		if !session.userinfo.IsAdmin {
			// The user isn't admin
			http.Error(w, "This operation requires admin", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, req)
	})
}

// wrapRequireAuth adds a layer that requires that the user
// has authenticated, either through Oauth2 or an API key.
func wrapRequireAuth(h httpHandlerWithAccessProfile) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// If authentication is not enabled in config, let the request through
		if !authRequired {
			h.ServeHTTP(w, req, &AccessProfile{isAdmin: true})
			return
		}
		session := getSessionFromRequest(req)
		if session == nil {
			// The user isn't logged in
			http.Error(w, "Not logged in", http.StatusUnauthorized)
			return
		}
		if session.AccessProfile == nil {
			// For some reason, the session is missing an access profile.
			// This is probably due to an error during login, and the user should re-authenticate.
			http.Error(w, "Not logged in", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, req, session.AccessProfile)
	})
}
