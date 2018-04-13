package main

import (
	"database/sql"
	"os"
	"testing"
)

func TestForwardConfirmReverseDNS(t *testing.T) {
	if os.Getenv("NONETWORK") != "" {
		t.Log("No network, skipping test")
		return
	}
	type dnstest struct {
		ipaddr string
		name   string
	}
	tests := []dnstest{
		dnstest{
			ipaddr: "129.240.2.42",
			name:   "ns2.uio.no",
		},
		dnstest{
			ipaddr: "2001:700:100:425::42",
			name:   "ns2.uio.no",
		},
		dnstest{
			ipaddr: "193.157.198.51",
			name:   "1x-193-157-198-51.uio.no",
		},
		dnstest{
			ipaddr: "192.168.0.1",
			name:   "",
		},
	}
	for _, test := range tests {
		result := forwardConfirmReverseDNS(test.ipaddr)
		if result != test.name {
			t.Errorf("Looked up %s, got \"%s\" but expected %s", test.ipaddr,
				result, test.name)
		}
	}
}

func TestHandleDNSchanges(t *testing.T) {
	if os.Getenv("NONETWORK") != "" || os.Getenv("NOPOSTGRES") != "" {
		t.Log("No network and/or Postgres, skipping test")
		return
	}
	// Create a database connection
	db := getDBconnForTesting(t)
	defer db.Close()
	// Set up some test data
	_, err := db.Exec("INSERT INTO ipranges(iprange,use_dns) " +
		"VALUES('129.240.0.0/16',true),('193.157.111.0/24',false)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO waiting_for_approval(ipaddr,hostname,approved) " +
		"VALUES('80.90.100.110', 'manual.example.com', true)," +
		"('80.90.100.112', 'manual.example.com', true)")
	if err != nil {
		t.Fatal(err)
	}
	type testname struct {
		certfp     string
		ipAddress  string
		osHostname string
		hostname   sql.NullString
		expected   string
	}
	tests := []testname{
		testname{
			certfp:     "a",
			ipAddress:  "129.240.202.63",
			osHostname: "bottleneck.bestchoice.com",
			expected:   "callisto.uio.no",
		},
		testname{
			certfp:     "b",
			ipAddress:  "193.157.111.23",
			osHostname: "paperweight.withoutdns.com",
			expected:   "paperweight.withoutdns.com.local",
		},
		testname{
			certfp:     "e",
			ipAddress:  "193.157.111.55",
			osHostname: "paperweight.withoutdns.com", // same name as previous
			expected:   "paperweight.withoutdns.com.2.local",
		},
		testname{
			certfp:     "c",
			ipAddress:  "129.240.2.6",
			osHostname: "not-the-correct-name.no",
			expected:   "",
		},
		testname{
			certfp:     "d",
			ipAddress:  "129.240.2.6",
			osHostname: "ns1.uio.no",
			hostname:   sql.NullString{String: "ns1.uio.no", Valid: true},
			expected:   "ns1.uio.no",
		},
		testname{
			certfp:     "g",
			ipAddress:  "80.90.100.110", // outside ranges, manually approved
			osHostname: "foo",           // shouldn't matter
			expected:   "manual.example.com",
		},
		testname{
			certfp:     "h",
			ipAddress:  "80.90.100.112", // outside ranges, manually approved
			osHostname: "foo",           // shouldn't matter
			expected:   "",              // the name "manual.example.com" is already taken now
		},
	}
	for _, test := range tests {
		_, err = db.Exec("INSERT INTO hostinfo(certfp,ipaddr,os_hostname,hostname) "+
			"VALUES($1,$2,$3,$4)", test.certfp, test.ipAddress, test.osHostname,
			test.hostname)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Run the function
	job := handleDNSchangesJob{}
	job.Run(db)
	// Check the results
	for _, test := range tests {
		var hostname sql.NullString
		err = db.QueryRow("SELECT hostname FROM hostinfo WHERE certfp=$1",
			test.certfp).Scan(&hostname)
		if err != nil {
			t.Fatal(err)
		}
		if hostname.String != test.expected {
			t.Errorf("Got hostname \"%s\", expected %s", hostname.String,
				test.expected)
		}
	}
}
