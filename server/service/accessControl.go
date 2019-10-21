package main

import (
	"database/sql"
	"net"
	"net/http"
	"strings"
	"time"
)

// AccessProfile holds information about which hosts the user is allowed access to,
// and whether the user has admin rights.
type AccessProfile struct {
	isAdmin   bool
	groups    map[string]bool
	allGroups bool
	expires   time.Time
	readonly  bool
	ipranges  []net.IPNet
}

func (ap *AccessProfile) HasAccessToGroup(group string) bool {
	return ap.isAdmin || ap.allGroups || ap.groups[group]
}

func (ap *AccessProfile) HasAccessToAllGroups() bool {
	return ap.isAdmin || ap.allGroups
}

func (ap *AccessProfile) IsAdmin() bool {
	return ap.isAdmin
}

func (ap *AccessProfile) IsMemberOf(group string) bool {
	return ap.groups[group]
}

func (ap *AccessProfile) HasExpired() bool {
	return !ap.expires.IsZero() && time.Until(ap.expires) <= 0
}

func (ap *AccessProfile) IsReadonly() bool {
	return ap.readonly
}

func (ap *AccessProfile) CanBeUsedFrom(ipaddr net.IP) bool {
	if ap.ipranges == nil {
		return false
	}
	for _, r := range ap.ipranges {
		if r.Contains(ipaddr) {
			return true
		}
	}
	return false
}

func (ap *AccessProfile) AllowAllIPs() {
	ap.ipranges = make([]net.IPNet, 1)
	ap.ipranges[0] = net.IPNet{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(0, 128)}
}

func (ap *AccessProfile) AllowOnlyLocalhost() {
	ap.ipranges = []net.IPNet{
		net.IPNet{IP: net.IPv4(127, 0, 0, 1), Mask: net.IPv4Mask(255, 255, 255, 255)},
		net.IPNet{IP: net.ParseIP("::1"), Mask: net.CIDRMask(1, 128)},
	}
}

// GetGroupListForSQLWHERE returns a list like: 'foo','bar','baz'
// If you use this function, you must first check if the user has access to all groups,
// because then you shouldn't use this function in a WHERE clause at all.
func (ap *AccessProfile) GetGroupListForSQLWHERE() string {
	var sql strings.Builder
	sql.WriteString("'")
	frist := true
	for k := range ap.groups {
		if !frist {
			sql.WriteString("','")
		}
		sql.WriteString(k)
		frist = false
	}
	sql.WriteString("'")
	return sql.String()
}

func GenerateAccessProfileForUser(isAdmin bool, groups []string) *AccessProfile {
	ap := new(AccessProfile)
	ap.isAdmin = isAdmin
	ap.groups = make(map[string]bool, len(groups))
	for _, g := range groups {
		ap.groups[g] = true
	}
	return ap
}

// ------------------------- http helpers -----------

type httpHandlerWithAccessProfile interface {
	ServeHTTP(http.ResponseWriter, *http.Request, *AccessProfile)
}

type httpHandlerWithAccessProfileFunc func(http.ResponseWriter, *http.Request, *AccessProfile)

func (f httpHandlerWithAccessProfileFunc) ServeHTTP(w http.ResponseWriter, r *http.Request, ap *AccessProfile) {
	f(w, r, ap)
}

// wrapRequireAdmin adds a layer that requires that the user has administrative rights,
// in addition to being authenticated, either through Oauth2 or an API key.
func wrapRequireAdmin(h http.Handler, db *sql.DB) http.Handler {
	return wrapRequireAuth(
		httpHandlerWithAccessProfileFunc(
			func(w http.ResponseWriter, req *http.Request, ap *AccessProfile) {
				if !ap.IsAdmin() {
					// The user isn't admin
					http.Error(w, "This operation requires admin", http.StatusForbidden)
					return
				}
				h.ServeHTTP(w, req)
			}),
		db)
}

// wrapRequireAuth adds a layer that requires that the user
// has authenticated, either through Oauth2 or an API key.
func wrapRequireAuth(h httpHandlerWithAccessProfile, db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// If authentication is not enabled in config, let the request through
		if !config.AuthRequired {
			h.ServeHTTP(w, req, &AccessProfile{isAdmin: true})
			return
		}
		var ap *AccessProfile
		apikey := GetAPIKeyFromRequest(req)
		if apikey != "" {
			// An API key overrides any session (these aren't supposed to be used together anyway)
			var err error
			ap, err = GetAccessProfileForAPIkey(apikey, db, nil)
			if err != nil {
				http.Error(w, "Error while composing the ACL for the API key:\n"+err.Error(),
					http.StatusInternalServerError)
				return
			}
			if ap == nil {
				http.Error(w, "No APIkey found with that ID.", http.StatusUnauthorized)
				return
			}
			// API keys can be restricted further
			if ap.HasExpired() {
				http.Error(w, "This key has expired", http.StatusForbidden)
				return
			}
			if !ap.CanBeUsedFrom(getRealRemoteAddr(req)) {
				http.Error(w, "This key can only be used from certain ip addresses", http.StatusForbidden)
				return
			}
			if ap.IsReadonly() && req.Method != httpGET {
				http.Error(w, "This key can only be used for GET requests", http.StatusForbidden)
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
				// If the session is missing an access profile, it is probably due to
				// an error during login, and the user should re-authenticate.
				http.Error(w, "Not logged in", http.StatusUnauthorized)
				return
			}
			ap = session.AccessProfile
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
