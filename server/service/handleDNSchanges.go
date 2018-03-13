package main

import (
	"database/sql"
	"log"
	"net"
	"time"
)

type handleDNSchangesJob struct{}

func init() {
	RegisterJob(handleDNSchangesJob{})
}

func (j handleDNSchangesJob) HowOften() time.Duration {
	return time.Minute * 5
}

// Look for hostinfo rows where dnsttl is null or expired,
// or where hostname is null.
// Perform the naming algorithm, and update hostname (and ttl) in the table.
func (j handleDNSchangesJob) Run(db *sql.DB) {
	// This function is structured so it uses only 1 database connection,
	// to facilitate unit testing with a temp schema.
	rows, err := db.Query("SELECT certfp,ipaddr,os_hostname FROM hostinfo " +
		"WHERE hostname is null OR dnsttl is null OR dnsttl < now()")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	type mRow struct {
		certfp, ipaddr, osHostname sql.NullString
	}
	list := make([]mRow, 0, 100)
	for rows.Next() {
		var c, ip, name sql.NullString
		err = rows.Scan(&c, &ip, &name)
		if err != nil {
			log.Fatal(err)
		}
		list = append(list, mRow{certfp: c, ipaddr: ip, osHostname: name})
	}
	rows.Close()
	for _, m := range list {
		var hostname string
		hostname, err = nameMachine(db, m.ipaddr.String, m.osHostname.String, m.certfp.String)
		if err != nil {
			log.Fatal(err)
		}
		if hostname == "" {
			continue
		}
		_, err = db.Exec("UPDATE hostinfo SET hostname=$1, dnsttl=now()+interval'1h' "+
			"WHERE certfp=$2", hostname, m.certfp.String)
		if err != nil {
			log.Printf("Trying to set hostname=\"%s\" for cert %s",
				hostname, m.certfp.String)
			log.Fatal(err)
		}
	}
	if err = rows.Err(); err != nil {
		log.Fatal(err)
	}
}

// This method tries to determine what hostname a machine has.
// Sometimes there's conflicting data, for example if DNS gives a different
// answer than the machine itself.
// This method is used from several parts of the software, to keep the logic
// in one place only.
func nameMachine(db *sql.DB, ipAddress string, osHostname string, certfp string) (string, error) {
	// 1. First, see if the ip address is within one of the ip ranges
	//    where DNS should be used.
	var count int
	err := db.QueryRow("SELECT count(*) FROM ipranges WHERE $1 <<= iprange "+
		"AND use_dns", ipAddress).Scan(&count)
	if err != nil {
		return "", err
	}
	if count > 0 {
		// Yes, use DNS.
		hostname := forwardConfirmReverseDNS(ipAddress)
		if hostname == "" {
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
			"hostname=os_hostname AND hostname=$1 AND certfp != $2",
			hostname, certfp).Scan(&count)
		if err != nil {
			return "", err
		}
		if count > 0 {
			// There's another machine that owns the name. Let it keep it.
			return "", nil
		}
		return hostname, nil
	}
	// The ip address is outside ranges where we can use DNS.
	// Check to see if the ip address qualifies for automatic naming.
	count = 0
	err = db.QueryRow("SELECT count(*) FROM ipranges WHERE $1 <<= iprange"+
		" AND NOT use_dns", ipAddress).Scan(&count)
	if err != nil {
		return "", err
	}
	if count > 0 {
		// Create a hostname based on OS + a suffix then.
		return osHostname + ".local", nil
	}
	return "", nil
}

// returns hostname or empty string
func forwardConfirmReverseDNS(ipAddress string) string {
	// First, look up the ip address and get a list of hostnames
	names, err := net.LookupAddr(ipAddress)
	if err != nil {
		return ""
	}
	// Forward-confirm-reverse
	for _, name := range names {
		// Strip trailing dot
		if name[len(name)-1] == '.' {
			name = name[0 : len(name)-1]
		}
		// Look up ip adresses for name
		addrs, err := net.LookupHost(name)
		if err != nil {
			return ""
		}
		for _, ip := range addrs {
			if ip == ipAddress {
				return name
			}
		}
	}
	return ""
}
