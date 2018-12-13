package main

import (
	"net/http"
	"strings"
)

type APIkey struct {
	key string
}

func (a *APIkey) String() string {
	return a.key
}

func GetAPIKeyFromRequest(req *http.Request) *APIkey {
	auth := strings.SplitN(req.Header.Get("Authorization"), " ", 2)
	if len(auth) == 2 && strings.ToLower(auth[0]) == "apikey" {
		return &APIkey{key: auth[1]}
	}
	return nil
}

func GetAccessProfileForAPIkey(key APIkey) *AccessProfile {
	//TODO implement

	// 1. Read the entry from the database table
	// 2. Call GenerateAccessProfileForUser on the ownerid to generate an accessprofile
	// 3. Call buildSQLwhere and perform a query to get a list of certs. UNION the two lists.
	// 4. Set the readonly flag, and the ip ranges, and the expiry date
	// 5. Cache the AccessProfile, so that subsequent calls to GetAccessProfileForAPIkey can quickly use it.

	return nil
}
