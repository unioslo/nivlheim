package main

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

type Session struct {
	userinfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	Oauth2AccessToken  *oauth2.Token
	Oauth2Config       *oauth2.Config
	Oauth2State        string
	RedirectAfterLogin string
}

var sessionMutex sync.RWMutex
var sessions map[string]*Session

func init() {
	sessions = make(map[string]*Session, 0)
}

const sessionCookieName = "nivlheimSession"

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
	return sessions[ID]
}

func newSession(w http.ResponseWriter, req *http.Request) *Session {
	// Create a new random session ID
	newID := randomStringID()
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
	if isLocal(req) {
		sessions = make(map[string]*Session, 0)
	}
	sessions[newID] = sPtr
	return sPtr
}

func deleteSession(req *http.Request) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	if isLocal(req) {
		sessions = make(map[string]*Session, 0)
	} else {
		cookie, err := req.Cookie(sessionCookieName)
		if err != nil {
			return
		}
		delete(sessions, cookie.Value)
	}
}

func randomStringID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func apiGetUserInfo(w http.ResponseWriter, req *http.Request) {
	sess := getSessionFromRequest(req)
	if sess == nil {
		var empty interface{}
		returnJSON(w, req, empty)
		return
	}
	returnJSON(w, req, sess.userinfo)
}
