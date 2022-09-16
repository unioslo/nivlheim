package main

import (
	"database/sql"
	"math/rand"
	"net"
	"net/http"
	"nivlheim/utility"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

type APIkey string

type apiKeyCacheElem struct {
	ap      *AccessProfile
	created time.Time
}

var apiKeyCacheMutex sync.RWMutex
var apiKeyCache map[string]apiKeyCacheElem

func init() {
	apiKeyCache = make(map[string]apiKeyCacheElem, 0)
}

func GetAPIKeyFromRequest(req *http.Request) APIkey {
	auth := strings.SplitN(req.Header.Get("Authorization"), " ", 2)
	if len(auth) == 2 && strings.ToLower(auth[0]) == "apikey" {
		return APIkey(auth[1])
	}
	return ""
}

func GenerateTemporaryAPIKey(ap *AccessProfile) APIkey {
	apiKeyCacheMutex.Lock()
	defer apiKeyCacheMutex.Unlock()
	key := utility.RandomStringID()
	apiKeyCache[key] = apiKeyCacheElem{ap: ap, created: time.Now()}
	return APIkey(key)
}

func GetAccessProfileForAPIkey(key APIkey, db *sql.DB, existingUserAP *AccessProfile) (*AccessProfile, error) {
	const cacheTimeMinutes = 10

	// 1. Check the cache
	apiKeyCacheMutex.RLock()
	c, ok := apiKeyCache[string(key)]
	apiKeyCacheMutex.RUnlock()
	if ok && time.Since(c.created) < time.Duration(cacheTimeMinutes)*time.Minute {
		return c.ap, nil
	}

	// 2. Read the entry from the database table
	var keyID int
	var expires pq.NullTime
	var readonly, allGroups sql.NullBool
	var groups []string
	err := db.QueryRow("SELECT keyid, groups, expires, readonly, all_groups "+
		"FROM apikeys WHERE key=$1", string(key)).
		Scan(&keyID, pq.Array(&groups), &expires, &readonly, &allGroups)
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}
		return nil, nil // No key was found, but this isn't an error
	}

	// 3. Some of the test scripts supply an AccessProfile to create various testing scenarios.
	//    In production, existingUserAP is always nil.
	var ap *AccessProfile
	if existingUserAP != nil {
		ap = existingUserAP
	} else {
		ap = new(AccessProfile)
	}

	// 4. Set various fields in the struct
	ap.readonly = readonly.Bool
	ap.isAdmin = false // keys can't give you admin rights. This may change in the future.
	ap.allGroups = allGroups.Bool
	if expires.Valid {
		ap.expires = expires.Time
	}
	ap.groups = make(map[string]bool, len(groups))
	for _, g := range groups {
		ap.groups[g] = true
	}

	// 5. Get the IP ranges
	rows, err := db.Query("SELECT iprange FROM apikey_ips WHERE keyID=$1", keyID)
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

	// 6. Cache the AccessProfile, so that subsequent calls to GetAccessProfileForAPIkey can quickly use it.
	apiKeyCacheMutex.Lock()
	defer apiKeyCacheMutex.Unlock()
	apiKeyCache[string(key)] = apiKeyCacheElem{ap: ap, created: time.Now()}

	// 7. Purge expired keys from the cache sometimes
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

func invalidateCacheForKey(key string) {
	apiKeyCacheMutex.Lock()
	defer apiKeyCacheMutex.Unlock()
	delete(apiKeyCache, key)
}
