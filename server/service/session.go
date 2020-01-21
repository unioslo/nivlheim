package main

import (
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/unioslo/nivlheim/server/service/utility"
	"golang.org/x/oauth2"
)

// Session holds session data for interactive user sessions (people, not scripts)
type Session struct {
	userinfo struct {
		Name         string   `json:"name"`
		Username     string   `json:"username"`
		IsAdmin      bool     `json:"isAdmin"`
		Groups       []string `json:"groups"`
		PrimaryGroup string   `json:"primaryGroup"`
	}
	userID             string
	Oauth2AccessToken  *oauth2.Token
	Oauth2Config       *oauth2.Config
	Oauth2State        string
	RedirectAfterLogin string
	lastUsed           time.Time
	mutex              sync.RWMutex
	AccessProfile      *AccessProfile
}

var sessionMutex sync.RWMutex
var sessions map[string]*Session

func init() {
	sessions = make(map[string]*Session, 0)
}

const sessionCookieName = "nivlheimSession"

// GetSessionFromRequest returns the session object
// associated with the http request, if there is any.
// Returns nil otherwise.
// This function does not create a new session.
func getSessionFromRequest(req *http.Request) *Session {
	if isLocal(req) {
		// Browsers have non-standard behavior with cookies toward localhost,
		// so session mgmt with cookies doesn't work when developing locally.
		// For development, let's just return the one and only session anyway.
		for _, sPtr := range sessions {
			return sPtr
		}
		return nil
	}
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil {
		// No session cookie found
		return nil
	}
	ID := cookie.Value
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()
	session, found := sessions[ID]
	if !found {
		return nil
	}
	session.mutex.Lock()
	defer session.mutex.Unlock()
	session.lastUsed = time.Now()
	return session
}

// HasSessionCookie provides a way to test if a request has a session cookie.
// Note that this doesn't tell you whether the session is valid or even exists.
func HasSessionCookie(req *http.Request) bool {
	_, err := req.Cookie(sessionCookieName)
	return err == nil
}

// newSession creates a new Session object, stores it for later,
// and sets a cookie with the session ID in the http response.
// It returns the new Session object.
func newSession(w http.ResponseWriter, req *http.Request) *Session {
	// Create a new random session ID
	newID := utility.RandomStringID()
	// Set a cookie with the session ID
	cookie := http.Cookie{
		Name:     sessionCookieName,
		Value:    newID,
		Expires:  time.Now().Add(8 * time.Hour),
		HttpOnly: true,
		Secure:   true,
		Path:     "/api",
	}
	http.SetCookie(w, &cookie)
	// Create a new session struct
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	sPtr := new(Session)
	sPtr.lastUsed = time.Now()
	if isLocal(req) {
		// If local connection, assume development environment.
		// Make sure there's only one active session
		sessions = make(map[string]*Session, 0)
	}
	sessions[newID] = sPtr
	return sPtr
}

func deleteSession(req *http.Request) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	if isLocal(req) {
		// If local connection, assume development environment.
		// There's only supposed to be one session, so delete all.
		sessions = make(map[string]*Session, 0)
	} else {
		cookie, err := req.Cookie(sessionCookieName)
		if err != nil {
			return
		}
		delete(sessions, cookie.Value)
	}
}

// API call /api/vx/userinfo
func apiGetUserInfo(w http.ResponseWriter, req *http.Request) {
	if !config.AuthRequired {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write([]byte("{\"authDisabled\":true,\"isAdmin\":true}"))
		return
	}
	sess := getSessionFromRequest(req)
	if sess == nil || sess.userID == "" {
		var empty interface{}
		returnJSON(w, req, empty)
		return
	}
	returnJSON(w, req, sess.userinfo)
}

// Job
type cleanupSessionsJob struct{}

func init() {
	RegisterJob(cleanupSessionsJob{})
}

func (job cleanupSessionsJob) HowOften() time.Duration {
	return time.Minute * 29
}

func (job cleanupSessionsJob) Run(db *sql.DB) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	for id, sPtr := range sessions {
		sPtr.mutex.RLock()
		//TODO configurable session timeout
		if time.Since(sPtr.lastUsed) > time.Duration(8)*time.Hour {
			delete(sessions, id)
		}
		sPtr.mutex.RUnlock()
	}
}
