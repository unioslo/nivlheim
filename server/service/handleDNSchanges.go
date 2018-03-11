package main

import (
	"database/sql"
	"net"
	"net/http"
	"time"
)

type handleDNSchangesJob struct{}

func init() {
	RegisterJob(handleDNSchangesJob{})
}

func (_ handleDNSchangesJob) HowOften() time.Duration {
	return time.Minute * 5
}

func (_ handleDNSchangesJob) Run(db *sql.DB) {
	// look for hostinfo rows where dnsttl is null or expired, or where hostname is null

}

// This method tries to determine what hostname a machine has.
// Sometimes there's conflicting data, for example if DNS gives a different
// answer than the machine itself.
// This method is used from several parts of the software, to keep the logic
// in one place only.
func nameMachine(db *sql.DB, ipAddress net.IP, osHostname string, certfp string) (string, *httpError) {
	// 1. First, see if the ip address is within one of the ip ranges
	//    where DNS should be used.
	var count int
	err := db.QueryRow("SELECT count(*) FROM ipranges WHERE $1 <<= iprange "+
		"AND use_dns = true", ipAddress.String()).Scan(&count)
	if err != nil {
		return "", &httpError{code: http.StatusInternalServerError, message: err.Error()}
	}
	if count > 0 {
		// Yes, use DNS.
		names, err := net.LookupAddr(ipAddress.String())
		if err != nil {
			return "", &httpError{code: http.StatusInternalServerError, message: err.Error()}
		}
		// Forward-confirm-reverse
		hostname := ""
		confirmed := false
		for _, name := range names {
			addrs, err := net.LookupIP(name)
			if err != nil {
				return "", &httpError{code: http.StatusInternalServerError, message: err.Error()}
			}
			for _, ip := range addrs {
				if ip.Equal(ipAddress) {
					confirmed = true
					hostname = name
					break
				}
			}
			if confirmed {
				break
			}
		}
		if !confirmed {
			// If DNS lookup wasn't conclusive, it's best to do nothing.
			return "", nil
		}
		// So, we have a name. Is it what the OS thinks it is?
		if hostname == osHostname {
			// Then we must assume it is correct
			return hostname, nil
		}
		// The OS and DNS don't agree.
		// Well, is the name in use by any other machine at the moment?
		// One who's OS agrees with DNS?
		err = db.QueryRow("SELECT count(*) FROM hostinfo WHERE "+
			"hostname=os_hostname AND hostname=? AND certfp != ?",
			hostname, certfp).Scan(&count)
		if err != nil {
			return "", &httpError{code: http.StatusInternalServerError, message: err.Error()}
		}
		if count > 0 {
			// There's another machine that owns the name.
			// Then let's keep it
			return "", nil
		}
		return hostname, nil
	} else {
		// The ip address is outside ranges where we can use DNS.
		// Check to see if the ip address qualifies for automatic naming.
		count = 0
		err := db.QueryRow("SELECT count(*) FROM ipranges WHERE $1 <<= iprange "+
			"AND use_dns = false", ipAddress.String()).Scan(&count)
		if err != nil {
			return "", &httpError{code: http.StatusInternalServerError, message: err.Error()}
		}
		if count > 0 {
			// Create a hostname based on OS + a suffix then.
			return osHostname + ".local", nil
		}
	}
	return "", nil
}
