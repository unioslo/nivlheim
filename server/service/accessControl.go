package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AccessProfile holds information about what data and
// which admin functions the owner is allowed access to.
type AccessProfile struct {
	key     string
	certs   map[string]bool
	created time.Time
}

func (ap *AccessProfile) HasAccessTo(certfp string) bool {
	return false
}

func (ap *AccessProfile) IsAdmin() bool {
	return false
}

var mapMutex sync.RWMutex
var profiles map[string]*AccessProfile

type HttpError struct {
	Code    int
	Message string
}

func (h *HttpError) Error() string {
	return fmt.Sprintf("%d %s", h.Code, h.Message)
}

func GetAccessProfileFromRequest(req *http.Request) (*AccessProfile, *HttpError) {
	// Extract the API key from the request (it's in a http header)
	apiKey := getAPIKeyFromRequest(req)
	// (Alternatively) If there's a session cookie, get the user id from the session
	var userID, profileKey string
	if apiKey == nil {
		session := getSessionFromRequest(req)
		if session != nil {
			userID = session.userinfo.ID
			profileKey = userID
		} else {
			// If neither API key nor session, return no profile
			return nil, &HttpError{Code: http.StatusUnauthorized, Message: "Needs authentication"}
		}
	} else {
		profileKey = apiKey.String()
	}
	// See if there's already a cached access profile,
	// and that it hasn't expired. If so, return it.
	mapMutex.RLock()
	profile, ok := profiles[profileKey]
	mapMutex.RUnlock()
	if ok && time.Since(profile.created) < time.Minute*10 {
		return profile, nil
	}
	// 3. If we have a username, go directly to step 7.
	// 4. Verify the API key. Is it valid?
	// 5. Verify the API key limitations. Remote IP? Expiry date?
	// 6. Map the API key to a username
	// 7. Call the external service to retrieve access data for that user.
	// 8. Create a new AccessProfile and set a timestamp in it.
	// 9. Store the new profile in memory, either by APIkey or username.
	// A. Return the new AccessProfile.

	// Possible outcomes:
	// 1a. Here's an AccessProfile, based on your API key
	// 1b. Here's an AccessProfile, based on your user session
	// 2. The API key isn't valid
	// 3. The API key is valid, but can't be used for this request according to its limitations
	// 4. No API key was supplied
	// 5. The external service is down.
	// 6. Another error occurred

	//TODO
	// 1. Decide how this integrates with the API code. Wrapper?

	//mapMutex.RLock()
	//defer mapMutex.RUnlock()
	//return profiles[key]
	return nil, nil
}

func getAPIKeyFromRequest(req *http.Request) *APIkey {
	auth := strings.Split(req.Header.Get("Authorization"), " ")
	if len(auth) == 2 && auth[0] == "APIKEY" && auth[1] != "" {
		return &APIkey{key: auth[1]}
	}
	return nil
}

type APIkey struct {
	key string
}

func (a *APIkey) String() string {
	return a.key
}
