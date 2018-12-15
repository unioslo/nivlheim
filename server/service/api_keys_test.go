package main

import (
	"net"
	"net/http"
	"os"
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
		if key.String() != kt.expectedKey {
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
		"INSERT INTO apikeys(keyid,ownerid,expiry,readonly,hostlistparams) " +
			"VALUES('1000','foo',now()+interval '10 minutes',true,'')," +
			"      ('1001','foo',now()+interval '10 minutes',false,'osEdition=server')",
		"INSERT INTO apikey_ips(keyid,iprange) VALUES('1000','192.168.0.0/24')," +
			"('1000','123.123.0.0/16'),('1001','50.50.50.64/26')",
		"INSERT INTO hostinfo(certfp,hostname,os_edition) " +
			"VALUES('1111','foo.bar.no','workstation'),('2222','bar.baz.no','server')," +
			"      ('3333','nobody.example.com','server')",
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
		certs:   map[string]bool{"1111": true, "2222": true},
	}
	// Define some tests
	type aTest struct {
		keyid          string
		expectAccessTo []string
	}
	tests := []aTest{
		// The key 1000 doesn't have any particular restrictions on hosts
		{
			keyid:          "1000",
			expectAccessTo: []string{"1111", "2222"},
		},
		// The key 1001 restricts to osEdition=server, should only give access to bar.baz.no
		{
			keyid:          "1001",
			expectAccessTo: []string{"2222"},
		},
	}
	// Run the tests
	for testNum, theTest := range tests {
		prevAP := fakeUserAP
		ap, err := GetAccessProfileForAPIkey(APIkey{key: theTest.keyid}, db, &prevAP)
		if err != nil {
			t.Fatal(err)
		}
		if ap == nil {
			t.Errorf("Test %d: Didn't get an access profile at all", testNum+1)
			continue
		}
		for _, s := range theTest.expectAccessTo {
			if !ap.HasAccessTo(s) {
				t.Errorf("\nTest %d: Expected access to %s but NO", testNum+1, s)
			}
		}
		if len(ap.certs) > len(theTest.expectAccessTo) {
			t.Errorf("\nTest %d: Got more access than I should have.", testNum+1)
		}
	}
	// Test that it reads the correct readonly/ipranges/expiry from the database
	prevAP := fakeUserAP
	ap, err := GetAccessProfileForAPIkey(APIkey{key: "1000"}, db, &prevAP)
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
	if ap.expiry.IsZero() ||
		time.Until(ap.expiry)-time.Duration(10)*time.Minute > time.Duration(10)*time.Second {
		t.Errorf("Expire date/time seems off: %v", ap.expiry)
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
