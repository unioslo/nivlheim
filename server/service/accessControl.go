package main

import (
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AccessProfile holds information about which hosts the user is allowed access to,
// and whether the user has admin rights.
type AccessProfile struct {
	certs    map[string]bool
	isAdmin  bool
	ownerID  string
	expiry   time.Time
	readonly bool
	ipranges []net.IPNet
}

func (ap *AccessProfile) HasAccessTo(certfp string) bool {
	return ap.isAdmin || ap.certs[certfp]
}

func (ap *AccessProfile) IsAdmin() bool {
	return ap.isAdmin
}

func (ap *AccessProfile) HasExpired() bool {
	return !ap.expiry.IsZero() && time.Until(ap.expiry) <= 0
}

func (ap *AccessProfile) IsReadonly() bool {
	return ap.readonly
}

func (ap *AccessProfile) CanBeUsedFrom(ipaddr net.IP) bool {
	if ap.ipranges == nil || len(ap.ipranges) == 0 {
		return true
	}
	for _, r := range ap.ipranges {
		if r.Contains(ipaddr) {
			return true
		}
	}
	return false
}

func (ap *AccessProfile) GetSQLWHERE() string {
	//TODO this is temporary (bad) solution; will be removed later. See issue #67
	var sql strings.Builder
	sql.WriteString("'")
	frist := true
	for k := range ap.certs {
		if !frist {
			sql.WriteString("','")
		}
		sql.WriteString(k)
		frist = false
	}
	sql.WriteString("'")
	return sql.String()
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
	type scriptResultType struct {
		IsAdmin bool     `json:"isAdmin"`
		Certs   []string `json:"certs"`
	}
	var scriptResult scriptResultType
	err = json.Unmarshal(jsonbytes, &scriptResult)
	if err != nil {
		return nil, err
	}
	ap := new(AccessProfile)
	ap.certs = make(map[string]bool)
	for _, s := range scriptResult.Certs {
		ap.certs[s] = true
	}
	ap.isAdmin = scriptResult.IsAdmin
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
func wrapRequireAuth(h httpHandlerWithAccessProfile, db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		//TODO let local auth plugin through. handle this in a better way.
		//  When api keys become a thing, the system can generate a short-lived
		//  api key on the fly and pass it to the auth plugin. That key wouldn't
		//  even have to be saved to the database.
		if isLocal(req) {
			h.ServeHTTP(w, req, &AccessProfile{isAdmin: true})
			return
		}
		// If authentication is not enabled in config, let the request through
		if !authRequired {
			h.ServeHTTP(w, req, &AccessProfile{isAdmin: true})
			return
		}
		var ap *AccessProfile
		apikey := GetAPIKeyFromRequest(req)
		if apikey != nil {
			// An API key overrides any session (these aren't supposed to be used together anyway)
			var err error
			ap, err = GetAccessProfileForAPIkey(*apikey, db, nil)
			if err != nil {
				http.Error(w, "Error while composing the ACL for the API key:\n"+err.Error(),
					http.StatusInternalServerError)
				return
			}
		} else {
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
			ap = session.AccessProfile
		}
		if ap.IsReadonly() && req.Method != httpGET {
			http.Error(w, "This key can only be used for GET requests", http.StatusForbidden)
			return
		}
		if !ap.CanBeUsedFrom(getRealRemoteAddr(req)) {
			http.Error(w, "This key can only be used from certain ip addresses", http.StatusForbidden)
			return
		}
		if ap.HasExpired() {
			http.Error(w, "This key has expired", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, req, ap)
	})
}

// getRealRemoteAddr takes into account that a local webserver may be used
// as a proxy, in which case RemoteAddr becomes 127.0.0.1 and we have to
// look at the X-Forwarded-For header instead.
func getRealRemoteAddr(req *http.Request) net.IP {
	var remoteAddr = strings.SplitN(req.RemoteAddr, ":", 2)[0] // host:port
	ip := net.ParseIP(remoteAddr)
	if !ip.IsLoopback() {
		return ip
	}
	ff, ok := req.Header["X-Forwarded-For"]
	if ok {
		// The client can pass its own value for the X-Forwarded-For header,
		// which will then be the first element of the array.
		// But the last element of the array will always be the address
		// that contacted the last proxy server, which is probably what we want.
		return net.ParseIP(ff[len(ff)-1])
	}
	return ip // can only be loopback at this point
}
