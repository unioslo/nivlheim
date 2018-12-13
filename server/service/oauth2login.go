package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/usit-gd/nivlheim/server/service/utility"
	"golang.org/x/oauth2"
)

func startOauth2Login(w http.ResponseWriter, req *http.Request) {
	redirectAfterLogin := req.FormValue("redirect")
	if redirectAfterLogin == "" {
		redirectAfterLogin = req.Header.Get("Referer")
	}

	// Check if the user is already logged in.
	// If so, just redirect to whereever.
	session := getSessionFromRequest(req)
	if session != nil {
		if session.userinfo.ID == "" {
			// If there's a session but the user isn't logged in,
			// it might be a leftover from a failed login.
			// Then delete that session and start over.
			deleteSession(req)
			session = nil
		} else {
			http.Redirect(w, req, redirectAfterLogin, http.StatusTemporaryRedirect)
			return
		}
	}

	// Assemble the redirect url
	var host = req.Host
	fh, ok := req.Header["X-Forwarded-Host"]
	if ok {
		host = fh[0]
	}
	// There's no way to detect if the original request used https,
	// but the rpm sets up Apache httpd with SSL by default,
	// so let's assume https unless running in development mode.
	var s string
	if devmode {
		s = "http"
	} else {
		s = "https"
	}
	s += "://" + host + "/api/oauth2/redirect"

	// Oauth2 configuration
	conf := &oauth2.Config{
		ClientID:     oauth2ClientID,
		ClientSecret: oauth2ClientSecret,
		Scopes:       oauth2Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  oauth2AuthorizationEndpoint,
			TokenURL: oauth2TokenEndpoint,
		},
		RedirectURL: s,
	}

	// Create a new session
	session = newSession(w, req)
	session.RedirectAfterLogin = redirectAfterLogin
	session.Oauth2Config = conf

	// State is a token to protect the user from CSRF attacks.
	// You must always provide a non-empty string and validate that it
	// matches the the state query parameter on your redirect callback.
	session.Oauth2State = randomStringID()

	// Redirect user to consent page to ask for permission
	// for the scopes specified above in the config.
	url := conf.AuthCodeURL(session.Oauth2State)
	log.Printf("Oauth2: Redirecting to %s", url)
	http.Redirect(w, req, url, http.StatusTemporaryRedirect)
}

func handleOauth2Redirect(w http.ResponseWriter, req *http.Request) {
	// Retrieve the session
	session := getSessionFromRequest(req)
	if session == nil {
		http.Error(w, "Missing session.", http.StatusInternalServerError)
		return
	}

	// Validate the state value (CSRF protection)
	if session.Oauth2State != req.FormValue("state") {
		http.Error(w, "Invalid Oauth2 state!", http.StatusBadRequest)
		return
	}

	// Exchange the auth code for an access token.
	tok, err := session.Oauth2Config.Exchange(oauth2.NoContext, req.FormValue("code"))
	if err != nil {
		log.Printf("Oauth2 exchange: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	session.Oauth2AccessToken = tok

	// The HTTP client returned by conf.Client will refresh the token as necessary.
	client := session.Oauth2Config.Client(oauth2.NoContext, tok)

	// Retrieve user info
	res, err := client.Get(oauth2UserInfoEndpoint)
	if err != nil {
		log.Printf("Oauth2 UserInfo error: %v", err)
		http.Error(w, "Unable to retrieve user info from Oauth2 provider", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Error reading Userinfo request body: %s", err.Error())
		http.Error(w, "Error reading Userinfo from Oauth2 provider", http.StatusInternalServerError)
		return
	}
	if devmode {
		log.Printf("Oauth2: Userinfo: %s", string(body))
	}

	// Parse the JSON
	var userinfo interface{}
	err = json.Unmarshal(body, &userinfo)
	if err != nil {
		log.Printf("Error parsing Userinfo from Oauth2 provider: %s", err.Error())
		http.Error(w, "Error while parsing Userinfo from Oauth2 provider", http.StatusInternalServerError)
		return
	}

	// Store the interesting values in the session
	if utility.GetString(userinfo, "audience") != oauth2ClientID {
		log.Printf("Oauth2 audience mismatch")
		http.Error(w, "Oauth2 audience mismatch", http.StatusInternalServerError)
		return
	}
	session.userinfo.ID = utility.GetString(userinfo, "user.userid_sec.0")
	session.userinfo.Name = utility.GetString(userinfo, "user.name")

	// Generate an access profile for this user by calling an external service
	session.AccessProfile, err = GenerateAccessProfileForUser(session.userinfo.ID)
	if err != nil {
		log.Printf("Error while generating an access profile: %s", err.Error())
		http.Error(w, "Error while generating an access profile", http.StatusInternalServerError)
		return
	}
	session.userinfo.IsAdmin = session.AccessProfile.IsAdmin()

	// Redirect to the page set in redirectAfterLogin.
	log.Printf("Oauth2: Redirecting to %s", session.RedirectAfterLogin)
	http.Redirect(w, req, session.RedirectAfterLogin, http.StatusTemporaryRedirect)
}

func oauth2Logout(w http.ResponseWriter, req *http.Request) {
	deleteSession(req)
	http.Redirect(w, req, oauth2LogoutEndpoint, http.StatusTemporaryRedirect)
}
