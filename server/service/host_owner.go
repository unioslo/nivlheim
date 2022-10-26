package main

import (
	"database/sql"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type determineHostOwnershipJob struct{}

func (job determineHostOwnershipJob) HowOften() time.Duration {
	return time.Minute * 15
}

func init() {
	RegisterJob(determineHostOwnershipJob{})
}

func (job determineHostOwnershipJob) Run(db *sql.DB) {
	// If no plugin has been configured, there's nothing to do here
	if config.HostOwnerPluginURL == "" {
		return
	}

	// Find hosts where ownergroup is null, or where the information has expired
	rows, err := db.Query("SELECT hostname,certfp FROM hostinfo WHERE ownergroup IS NULL " +
		"OR ownergroup_ttl IS NULL OR ownergroup_ttl < now()")
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	type hostinfo struct {
		hostname string
		certfp   string
	}
	list := make([]hostinfo, 0) // Read it into this slice
	for rows.Next() {
		var hostname, certfp sql.NullString
		err = rows.Scan(&hostname, &certfp)
		if err != nil {
			log.Panic(err)
		}
		list = append(list, hostinfo{hostname: hostname.String, certfp: certfp.String})
	}
	if err = rows.Err(); err != nil {
		log.Panic(err)
	}
	rows.Close()
	// Create a temporary API key that the plugin can use if it needs to talk to Nivlheim
	pluginAccess := &AccessProfile{
		isAdmin:   false,
		allGroups: true,
		readonly:  true,
		expires:   time.Now().Add(time.Duration(10) * time.Minute),
	}
	pluginAccess.AllowOnlyLocalhost()
	tempKey := GenerateTemporaryAPIKey(pluginAccess)
	defer func() {
		// When the function exits, even if panic, make the key expire right away
		pluginAccess.expires = time.Now()
	}()
	// Loop through those hosts and call the plugin to determine ownership for each of them
	for _, host := range list {
		if devmode {
			log.Printf("Trying to determine owner group for %s", host.hostname)
		}
		postValues := url.Values{}
		postValues.Set("key", string(tempKey))
		postValues.Set("hostname", host.hostname)
		postValues.Set("certfp", host.certfp)
		resp, err := http.PostForm(config.HostOwnerPluginURL, postValues)
		if err != nil {
			log.Panic(err)
		}
		bytes, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Panic(err)
		}
		if devmode {
			log.Printf("Plugin responded: %d %s", resp.StatusCode, bytes)
		}
		// Check the http status
		if resp.StatusCode > 299 {
			// oops, the statuscode indicates an error
			log.Panicf("http status %d from host owner plugin", resp.StatusCode)
		}
		// Parse the response from the plugin. Should be one line of text
		// that only contains the owner group name, nothing else.
		ownerGroup := strings.TrimSpace(string(bytes))
		if ownerGroup != "" {
			db.Exec("UPDATE hostinfo SET ownergroup=$1, ownergroup_ttl=now()+interval '1 day' "+
				"WHERE certfp=$2", ownerGroup, host.certfp)
		}
	}
}
