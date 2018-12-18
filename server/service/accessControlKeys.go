package main

import (
	"database/sql"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

type APIkey struct {
	key string
}

func (a *APIkey) String() string {
	if a == nil {
		return ""
	}
	return a.key
}

type apiKeyCacheElem struct {
	ap      *AccessProfile
	created time.Time
}

var apiKeyCacheMutex sync.RWMutex
var apiKeyCache map[string]apiKeyCacheElem

func init() {
	apiKeyCache = make(map[string]apiKeyCacheElem, 0)
}

func GetAPIKeyFromRequest(req *http.Request) *APIkey {
	auth := strings.SplitN(req.Header.Get("Authorization"), " ", 2)
	if len(auth) == 2 && strings.ToLower(auth[0]) == "apikey" {
		return &APIkey{key: auth[1]}
	}
	return nil
}

func GetAccessProfileForAPIkey(key APIkey, db *sql.DB, existingUserAP *AccessProfile) (*AccessProfile, error) {
	const cacheTimeMinutes = 10

	// 0. Check the cache
	apiKeyCacheMutex.RLock()
	c, ok := apiKeyCache[key.String()]
	apiKeyCacheMutex.RUnlock()
	if ok && time.Since(c.created) < time.Duration(cacheTimeMinutes)*time.Minute {
		return c.ap, nil
	}

	// 1. Read the entry from the database table
	var ownerid, filter sql.NullString
	var expires pq.NullTime
	var readonly sql.NullBool
	err := db.QueryRow("SELECT ownerid, expires, readonly, filter "+
		"FROM apikeys WHERE key=$1", key.String()).
		Scan(&ownerid, &expires, &readonly, &filter)
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}
		return nil, nil // the key was not found, but this isn't an error
	}

	// 2. Call GenerateAccessProfileForUser on the ownerid to generate an accessprofile
	var ap *AccessProfile
	if existingUserAP != nil {
		ap = existingUserAP
	} else {
		ap, err = GenerateAccessProfileForUser(ownerid.String)
		if err != nil {
			return nil, err
		}
	}

	// 3. Call buildSQLwhere with the hostlist parameters,
	//    and perform a query to get a list of certs. UNION the two lists.
	if filter.Valid && strings.TrimSpace(filter.String) != "" {
		newMap := make(map[string]bool, len(ap.certs))
		where, args, httpErr := buildSQLWhere(filter.String, nil)
		if httpErr != nil {
			return nil, httpErr
		}
		rows, err := db.Query("SELECT certfp FROM hostinfo WHERE "+where, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var certfp sql.NullString
			err = rows.Scan(&certfp)
			if err != nil {
				return nil, err
			}
			if certfp.Valid && ap.certs[certfp.String] {
				newMap[certfp.String] = true
			}
		}
		rows.Close()
		ap.certs = newMap
	}

	// 4. Set the readonly flag, and the ip ranges, and the expiration date
	ap.readonly = readonly.Bool
	if expires.Valid {
		ap.expires = expires.Time
	}
	rows, err := db.Query("SELECT iprange FROM apikey_ips WHERE key=$1", key.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ap.ipranges = make([]net.IPNet, 0)
	for rows.Next() {
		var r string
		err = rows.Scan(&r)
		if err != nil {
			return nil, err
		}
		_, ipnet, err := net.ParseCIDR(r)
		if err != nil {
			return nil, err
		}
		ap.ipranges = append(ap.ipranges, *ipnet)
	}
	rows.Close()

	// 5. Cache the AccessProfile, so that subsequent calls to GetAccessProfileForAPIkey can quickly use it.
	apiKeyCacheMutex.Lock()
	defer apiKeyCacheMutex.Unlock()
	apiKeyCache[key.String()] = apiKeyCacheElem{ap: ap, created: time.Now()}

	// 6. Purge expired keys from the cache sometimes
	if rand.Intn(100) == 0 {
		// the mutex is already locked, so it's safe to modify the map
		for id, c := range apiKeyCache {
			if time.Since(c.created) > time.Duration(cacheTimeMinutes)*time.Minute {
				delete(apiKeyCache, id)
			}
		}
	}

	return ap, nil
}
