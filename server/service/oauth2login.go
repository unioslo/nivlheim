package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/usit-gd/nivlheim/server/service/utility"
	"golang.org/x/oauth2"
)

func startOauth2Login(w http.ResponseWriter, req *http.Request) {
	redirectAfterLogin := req.FormValue("redirect")
	if redirectAfterLogin == "" {
		redirectAfterLogin = req.Header.Get("Referer")
	}

	// Check if the user is already logged in.
	session := getSessionFromRequest(req)
	if session != nil {
		if session.userID == "" {
			// If there's a session but the user isn't logged in,
			// it might be a leftover from a failed login.
			// Then delete that session and start over.
			deleteSession(req)
			session = nil
		} else {
			// The user is already logged in. Just redirect to the given url.
			http.Redirect(w, req, redirectAfterLogin, http.StatusTemporaryRedirect)
			return
		}
	}

	// Assemble the redirect url that the Oauth2 provider will use to redirect back to us.
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
		ClientID:     config.Oauth2ClientID,
		ClientSecret: config.Oauth2ClientSecret,
		Scopes:       config.Oauth2Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  config.Oauth2AuthorizationEndpoint,
			TokenURL: config.Oauth2TokenEndpoint,
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
	session.Oauth2State = utility.RandomStringID()

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
	res, err := client.Get(config.Oauth2UserInfoEndpoint)
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

	// Parse the JSON with the userinfo
	var userinfo interface{}
	err = json.Unmarshal(body, &userinfo)
	if err != nil {
		log.Printf("Error parsing Userinfo from Oauth2 provider: %s", err.Error())
		http.Error(w, "Error while parsing Userinfo from Oauth2 provider", http.StatusInternalServerError)
		return
	}

	// ====----====----====----====----====----====----====----====----====----====----
	// The following code is specific to Feide. (www.feide.no)
	// Oauth2 providers format their userinfo object slightly differently.
	// If you wish to use another provider, you must add code to detect and parse that.
	// See also:
	// https://docs.feide.no/developer_oauth/technical_details/oauth_authentication.html

	// Feide: When using the userinfo endpoint to authenticate the user,
	// the application MUST verify that the audience property matches the client id of the application.
	if utility.GetString(userinfo, "audience") != config.Oauth2ClientID {
		log.Printf("Oauth2 audience mismatch")
		http.Error(w, "Oauth2 audience mismatch", http.StatusInternalServerError)
		return
	}

	// Store the interesting values from userinfo in the session
	session.userID = utility.GetString(userinfo, "user.userid_sec.0")
	session.userinfo.Name = utility.GetString(userinfo, "user.name")
	matches := regexp.MustCompile("feide:([\\w\\-]+)@").FindStringSubmatch(session.userID)
	if matches != nil {
		session.userinfo.Username = matches[1]
	}
	// ====----====----====----==== End of Feide-specific code ====----====----====----

	// If the config specifies an LDAP server, look up the user in LDAP
	if config.LDAPServer != "" && session.userinfo.Username != "" {
		user, err := LDAPLookupUser(session.userinfo.Username)
		if err != nil {
			log.Printf("Unable to LDAP: %v", err)
			http.Error(w, "Unable to perform LDAP lookup", http.StatusInternalServerError)
			return
		}
		// Also look up the "drift" user and add the groups from there.
		// This solution is probably only used by UiO,
		// but is likely to be harmless in other environments.
		user2, err := LDAPLookupUser(session.userinfo.Username + "-drift")
		if err != nil {
			log.Printf("Unable to LDAP: %v", err)
			http.Error(w, "Unable to perform LDAP lookup", http.StatusInternalServerError)
			return
		}
		if user2 != nil {
			// Add these groups to user's group list
			user.Groups = append(user.Groups, user2.Groups...)
		}
		// Remove duplicate entries
		user.Groups = utility.RemoveDuplicateStrings(user.Groups)
		// Sort the group list
		sort.Strings(user.Groups)
		session.userinfo.Groups = user.Groups

		// fuzzy logic to determine which group matches the primary affiliation best
		lowerCaseAff := strings.ToLower(user.PrimaryAffiliation)
		hit := -1
		minDist := 10000
		for i, g := range user.Groups {
			dist := LevenshteinDistance(strings.ToLower(g), lowerCaseAff)
			if hit == -1 || dist < minDist {
				hit = i
				minDist = dist
			}
		}
		if hit > -1 {
			session.userinfo.PrimaryGroup = user.Groups[hit]
		}

		// If the user is member of a special "admin" group, the user gets admin rights
		if config.LDAPAdminGroup != "" {
			for _, gname := range session.userinfo.Groups {
				if gname == config.LDAPAdminGroup {
					session.userinfo.IsAdmin = true
					break
				}
			}
		}
	}

	// Generate an access profile for this user
	session.AccessProfile = GenerateAccessProfileForUser(
		session.userinfo.IsAdmin, session.userinfo.Groups)

	// Redirect to the page set in redirectAfterLogin.
	log.Printf("Oauth2: Redirecting to %s", session.RedirectAfterLogin)
	http.Redirect(w, req, session.RedirectAfterLogin, http.StatusTemporaryRedirect)
}

func oauth2Logout(w http.ResponseWriter, req *http.Request) {
	deleteSession(req)
	http.Redirect(w, req, config.Oauth2LogoutEndpoint, http.StatusTemporaryRedirect)
}
