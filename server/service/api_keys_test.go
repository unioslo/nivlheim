package main

import (
	"database/sql"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestGetAPIKeyFromRequest(t *testing.T) {
	type keyTest struct {
		header      string
		expectedKey string
	}
	tests := []keyTest{
		{
			header:      "APIkey abcd",
			expectedKey: "abcd",
		},
		{
			header:      "aPiKeY abcd",
			expectedKey: "abcd",
		},
		{
			header:      "abcd",
			expectedKey: "",
		},
		{
			header:      "",
			expectedKey: "",
		},
	}
	for _, kt := range tests {
		req, err := http.NewRequest("GET", "/whatever", nil)
		if err != nil {
			t.Error(err)
			continue
		}
		req.RemoteAddr = "123.123.123.123"
		if kt.header != "" {
			req.Header.Add("Authorization", kt.header)
		}
		key := GetAPIKeyFromRequest(req)
		if string(key) != kt.expectedKey {
			t.Errorf("%s\nExpected API key %s, got %v", kt.header, kt.expectedKey, key)
		}
	}
}

func TestGetAccessProfileForAPIkey(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}
	db := getDBconnForTesting(t)
	defer db.Close()
	// Setup some data for testing
	setupStatements := []string{
		"INSERT INTO apikeys(key,ownergroup,expires,readonly,all_groups,groups) " +
			"VALUES('1000','foo',now()+interval '10 minutes',true,true,null)," +
			"      ('1001','foo',now()+interval '10 minutes',false,false,'{\"servergroup\"}')",
		"INSERT INTO apikey_ips(keyid,iprange) VALUES " +
			"((SELECT keyid FROM apikeys WHERE key='1000'),'192.168.0.0/24')," +
			"((SELECT keyid FROM apikeys WHERE key='1000'),'123.123.0.0/16')," +
			"((SELECT keyid FROM apikeys WHERE key='1001'),'50.50.50.64/26')",
		"INSERT INTO hostinfo(certfp,hostname,os_edition,ownergroup) " +
			"VALUES('1111','foo.bar.no','workstation','workgroup')," +
			"      ('2222','bar.baz.no','server','servergroup')," +
			"      ('3333','nobody.example.com','server','othergroup')",
		"INSERT INTO customfields(name) VALUES('duck'),('town')",
		"INSERT INTO hostinfo_customfields(certfp,fieldid,value) " +
			"VALUES('1111',1,'donald'),('2222',1,'gladstone')," +
			"      ('1111',2,'duckville'),('2222',2,'duckville')",
	}
	for _, sql := range setupStatements {
		_, err := db.Exec(sql)
		if err != nil {
			t.Fatalf("%s\n%v", sql, err)
		}
	}
	fakeUserAP := AccessProfile{
		isAdmin: false,
		groups:  map[string]bool{"firstgroup": true, "secondgroup": true},
	}
	// Define some tests
	type aTest struct {
		key            string
		expectAccessTo []string
	}
	tests := []aTest{
		// The key 1000 doesn't have any particular restrictions on groups
		{
			key:            "1000",
			expectAccessTo: []string{"1111", "2222", "3333"},
		},
		// The key 1001 is restricted to the group "servergroup", should only give access to bar.baz.no
		{
			key:            "1001",
			expectAccessTo: []string{"2222"},
		},
	}
	// Run the tests
	for testNum, theTest := range tests {
		prevAP := fakeUserAP
		ap, err := GetAccessProfileForAPIkey(APIkey(theTest.key), db, &prevAP)
		if err != nil {
			t.Fatal(err)
		}
		if ap == nil {
			t.Errorf("Test %d: Didn't get an access profile at all", testNum+1)
			continue
		}
		// Loop through all hosts and check for access
		rows, _ := db.Query("SELECT certfp,ownergroup FROM hostinfo")
		defer rows.Close()
		for rows.Next() {
			var cfp, gr string
			rows.Scan(&cfp, &gr)
			shouldHaveAccess := false
			for _, s := range theTest.expectAccessTo {
				if s == cfp {
					shouldHaveAccess = true
					break
				}
			}
			hasAccess := ap.HasAccessToGroup(gr)
			if shouldHaveAccess && !hasAccess {
				t.Errorf("\nTest %d: Expected access to %s, but I didn't get it :-(", testNum+1, gr)
			} else if hasAccess && !shouldHaveAccess {
				t.Errorf("\nTest %d: Got access to %s, even though I shouldn't!", testNum+1, gr)
			}
		}
	}
	// Test that it reads the correct readonly/ipranges/expires from the database
	prevAP := fakeUserAP
	ap, err := GetAccessProfileForAPIkey(APIkey("1000"), db, &prevAP)
	if err != nil {
		t.Fatal(err)
	}
	if !ap.IsReadonly() {
		t.Error("Didn't load readonly flag correctly.")
	}
	if len(ap.ipranges) != 2 || !testIPContains(ap.ipranges, "192.168.0.0/24") ||
		!testIPContains(ap.ipranges, "123.123.0.0/16") {
		t.Errorf("Didn't load IP ranges correctly: %v", ap.ipranges)
	}
	if ap.expires.IsZero() ||
		time.Until(ap.expires)-time.Duration(10)*time.Minute > time.Duration(10)*time.Second {
		t.Errorf("Expiration date/time seems off: %v", ap.expires)
	}
	// Test what happens if the provided API key doesn't exist in the database
	ap, err = GetAccessProfileForAPIkey(APIkey("nonexistent"), db, nil)
	if ap != nil || err != nil {
		t.Errorf("Tried to GetAccessProfileForAPIkey for non-existent key, got %v %v", ap, err)
	}
}

func testIPContains(s []net.IPNet, e string) bool {
	for _, a := range s {
		if a.String() == e {
			return true
		}
	}
	return false
}

func TestKeyCRUD(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}
	db := getDBconnForTesting(t)
	defer db.Close()
	rand.Seed(1)
	tests := []apiCall{
		// create a key with default values
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "ownergroup=mygroup",
			expectStatus:  http.StatusCreated,
		},
	}
	muxer := createAPImuxer(db, true)
	testAPIcalls(t, muxer, tests)

	// get the key
	var keyIDnum int
	var keyID string
	err := db.QueryRow("SELECT keyid FROM apikeys LIMIT 1").Scan(&keyIDnum)
	if err == sql.ErrNoRows {
		t.Fatal("Failed to create a key.")
	}
	if err != nil {
		t.Fatal(err)
	}
	keyID = strconv.Itoa(keyIDnum)

	// make more tests
	tests = []apiCall{
		// read the key list
		{
			methodAndPath: "GET /api/v2/keys?fields=keyID,readonly,groups",
			expectStatus:  http.StatusOK,
			expectJSON:    "[{\"keyID\":" + keyID + ",\"readonly\":true,\"groups\":[],\"allGroups\":false}]",
		},
		// update a key
		{
			methodAndPath: "PUT /api/v2/keys/" + keyID,
			body:          "comment=foo&expires=2020-12-24T18:00:00%2B01:00&readonly=no&groups=aa,bb",
			expectStatus:  http.StatusNoContent,
		},
		// read one key
		{
			methodAndPath: "GET /api/v2/keys/" + keyID + "?fields=comment,readonly,expires,groups",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"comment\":\"foo\",\"readonly\":false,\"expires\":\"2020-12-24T19:00:00+02:00\",\"groups\":[\"aa\",\"bb\"],\"allGroups\":false}",
		},
		// try to read a non-existent key
		{
			methodAndPath: "GET /api/v2/keys/123?fields=comment,readonly,expires",
			expectStatus:  http.StatusNotFound,
		},
		// update the key with some ip ranges. Also tests that short date format is allowed.
		{
			methodAndPath: "PUT /api/v2/keys/" + keyID,
			body:          "ipranges=192.168.1.0/24,172.16.0.0/20&comment=gep&expires=2019-12-13",
			expectStatus:  http.StatusNoContent,
		},
		// try to update a non-existent key
		{
			methodAndPath: "PUT /api/v2/keys/817198372",
			body:          "comment=foo",
			expectStatus:  http.StatusNotFound,
		},
		// read the key, verify the ip ranges
		{
			methodAndPath: "GET /api/v2/keys/" + keyID + "?fields=ipranges,comment",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"ipRanges\":[\"192.168.1.0/24\",\"172.16.0.0/20\"],\"comment\":\"gep\"}",
		},
		// delete the key
		{
			methodAndPath: "DELETE /api/v2/keys/" + keyID,
			expectStatus:  http.StatusNoContent,
		},
		// delete the key again (should not work)
		{
			methodAndPath: "DELETE /api/v2/keys/" + keyID,
			expectStatus:  http.StatusNotFound,
		},
		// list the keys (now empty)
		{
			methodAndPath: "GET /api/v2/keys?fields=key,readonly",
			expectStatus:  http.StatusOK,
			expectJSON:    "[]",
		},
		// create a new key with some ip ranges
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "ipRanges=192.168.1.0/24,172.16.0.0/20&ownerGroup=mygroup",
			expectStatus:  http.StatusCreated,
		},
		// read it back
		{
			methodAndPath: "GET /api/v2/keys?fields=ipranges",
			expectStatus:  http.StatusOK,
			expectJSON:    "[{\"ipRanges\":[\"192.168.1.0/24\",\"172.16.0.0/20\"]}]",
		},
		// create a new key with an invalid ip range (bits set to the right of the netmask)
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "ipRanges=192.168.1.3/24&ownerGroup=mygroup",
			expectStatus:  http.StatusBadRequest,
		},
		// create a new key with invalid ip ranges
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "ipRanges=192.168.345.765/32&ownerGroup=mygroup",
			expectStatus:  http.StatusBadRequest,
		},
		// post garbage
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "%#)(/¤&)(#/¤&()#¤",
			expectStatus:  http.StatusBadRequest,
		},
	}
	testAPIcalls(t, muxer, tests)

	err = db.QueryRow("SELECT max(keyid) FROM apikeys").Scan(&keyIDnum)
	if err != nil {
		t.Fatal(err)
	}
	keyIDnum++ // The next key to be created will have the next ID value in the sequence
	keyID = strconv.Itoa(keyIDnum)

	// Test some API calls with a non-privileged (not admin) user
	userap := GenerateAccessProfileForUser(false, []string{"mygroup"})
	userap.AllowAllIPs()
	otherUserAP := GenerateAccessProfileForUser(false, []string{"othergroup"})
	otherUserAP.AllowAllIPs()
	tests = []apiCall{
		// Try to create a key that has access to all groups, should fail
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "ownergroup=mygroup&allGroups=true",
			accessProfile: userap,
			expectStatus:  http.StatusForbidden,
		},
		// Create a key without allGroups, should work
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "ownergroup=mygroup",
			accessProfile: userap,
			expectStatus:  http.StatusCreated,
		},
		// Try to extend the key's access to all groups, should fail
		{
			methodAndPath: "PUT /api/v2/keys/" + keyID,
			body:          "allGroups=true",
			accessProfile: userap,
			expectStatus:  http.StatusForbidden,
		},
		// Try to extend the key's access to groups that you don't have access to, should fail
		{
			methodAndPath: "PUT /api/v2/keys/" + keyID,
			body:          "groups=mygroup,theirGroup,anotherGroup",
			accessProfile: userap,
			expectStatus:  http.StatusForbidden,
		},
		// Try to delete a key you don't have access to, should fail
		{
			methodAndPath: "DELETE /api/v2/keys/" + keyID,
			accessProfile: otherUserAP,
			expectStatus:  http.StatusForbidden,
		},
		// Try to update a key you don't have access to, should fail
		{
			methodAndPath: "PUT /api/v2/keys/" + keyID,
			body:          "ownergroup=othergroup,readonly=false,expires=2050-04-30",
			accessProfile: otherUserAP,
			expectStatus:  http.StatusForbidden,
		},
		// Delete that key
		{
			methodAndPath: "DELETE /api/v2/keys/" + keyID,
			accessProfile: userap,
			expectStatus:  http.StatusNoContent,
		},
		// Be admin and create a key with all groups access
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "ownergroup=mygroup&allGroups=true",
			expectStatus:  http.StatusCreated,
		},
		// Read it back and verify that it has allGroups set
		{
			methodAndPath: "GET /api/v2/keys/" + strconv.Itoa(keyIDnum+1) + "?fields=groups",
			expectStatus:  http.StatusOK,
			expectJSON:    "{\"groups\":[],\"allGroups\":true}",
		},
		// Try to delete a key with all groups access as a non-admin user, should fail
		{
			methodAndPath: "DELETE /api/v2/keys/" + strconv.Itoa(keyIDnum+1),
			accessProfile: userap,
			expectStatus:  http.StatusForbidden,
		},
	}

	testAPIcalls(t, muxer, tests)
}

func TestAccessToEditingAPIkeys(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}
	db := getDBconnForTesting(t)
	defer db.Close()

	firstUser := AccessProfile{isAdmin: false, groups: map[string]bool{"firstGroup": true}}
	firstUser.AllowAllIPs()
	secondUser := AccessProfile{isAdmin: false, groups: map[string]bool{"secondGroup": true}}
	secondUser.AllowAllIPs()

	tests := []apiCall{
		// create two keys owned by two different groups
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "comment=first&ownerGroup=firstGroup",
			expectStatus:  http.StatusCreated,
			accessProfile: &firstUser,
		},
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "comment=second&ownerGroup=secondGroup",
			expectStatus:  http.StatusCreated,
			accessProfile: &secondUser,
		},
		// try to create a key for a group the user is not a member of, it should fail
		{
			methodAndPath: "POST /api/v2/keys",
			body:          "comment=wrong&ownerGroup=firstGroup",
			expectStatus:  http.StatusForbidden,
			accessProfile: &secondUser,
		},
		// read the key list, verify that you only see your own keys
		{
			methodAndPath: "GET /api/v2/keys?fields=comment",
			expectStatus:  http.StatusOK,
			expectJSON:    "[{\"comment\":\"first\"}]",
			accessProfile: &firstUser,
		},
		{
			methodAndPath: "GET /api/v2/keys?fields=comment",
			expectStatus:  http.StatusOK,
			expectJSON:    "[{\"comment\":\"second\"}]",
			accessProfile: &secondUser,
		},
		// if you're unauthorized, you shouldn't see any of them
		{
			methodAndPath: "GET /api/v2/keys?fields=comment",
			expectStatus:  http.StatusUnauthorized,
			runAsNotAuth:  true,
		},
		// try to read a key you don't own
		{
			methodAndPath: "GET /api/v2/keys/1?fields=comment",
			expectStatus:  http.StatusForbidden,
			accessProfile: &secondUser,
		},
		// try to modify a key you don't own
		{
			methodAndPath: "PUT /api/v2/keys/1",
			body:          "comment=ha-ha",
			expectStatus:  http.StatusForbidden,
			accessProfile: &secondUser,
		},
		// try to delete a key you don't own
		{
			methodAndPath: "DELETE /api/v2/keys/1",
			expectStatus:  http.StatusForbidden,
			accessProfile: &secondUser,
		},
	}
	muxer := createAPImuxer(db, true)
	testAPIcalls(t, muxer, tests)
}
