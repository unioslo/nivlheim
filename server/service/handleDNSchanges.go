package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/lib/pq"
	"github.com/usit-gd/nivlheim/server/service/utility"
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
	rows, err := db.Query("SELECT certfp,ipaddr,os_hostname,lastseen " +
		"FROM hostinfo " +
		"WHERE hostname is null OR dnsttl is null OR dnsttl < now()")
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	type mRow struct {
		certfp, ipaddr, osHostname sql.NullString
		lastseen                   pq.NullTime
	}
	list := make([]mRow, 0, 100)
	for rows.Next() {
		var c, ip, name sql.NullString
		var lseen pq.NullTime
		err = rows.Scan(&c, &ip, &name, &lseen)
		if err != nil {
			log.Panic(err)
		}
		list = append(list, mRow{certfp: c, ipaddr: ip, osHostname: name,
			lastseen: lseen})
	}
	if err = rows.Err(); err != nil {
		log.Panic(err)
	}
	rows.Close()
	for _, m := range list {
		err = utility.RunInTransaction(db, func(tx *sql.Tx) error {
			var hostname string
			hostname, err = nameMachine(tx, m.ipaddr.String, m.osHostname.String,
				m.certfp.String, m.lastseen.Time)
			if err != nil {
				log.Panic(err)
			}
			if hostname == "" {
				return errors.New("Failed to name " + m.osHostname.String + " (" + m.ipaddr.String + ")")
			}
			_, err = tx.Exec("DELETE FROM hostinfo WHERE hostname=$1 AND certfp!=$2",
				hostname, m.certfp.String)
			if err != nil {
				log.Panic(err)
			}
			_, err = tx.Exec("UPDATE hostinfo SET hostname=$1, dnsttl=now()+interval'1h' "+
				"WHERE certfp=$2", hostname, m.certfp.String)
			if err != nil {
				log.Printf("Trying to set hostname=\"%s\" for cert %s",
					hostname, m.certfp.String)
				log.Panic(err)
			}
			return nil
		})
		if err != nil {
			log.Print(err)
		}
	}
}

// This method tries to determine what hostname a machine has.
// Sometimes there's conflicting data, for example if DNS gives a different
// answer than the machine itself.
// This method is used from several parts of the software, to keep the logic
// in one place only.
func nameMachine(tx *sql.Tx, ipAddress string, osHostname string, certfp string,
	lastseen time.Time) (string, error) {
	// 1. First, see if the ip address is within one of the ip ranges
	//    where DNS should be used.
	var count int
	err := tx.QueryRow("SELECT count(*) FROM ipranges WHERE $1 <<= iprange "+
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
		// Ok, we have a hostname. Is it in use by another row that has the same ip address
		// (which means same claim to it by DNS) and is more recent?
		err = tx.QueryRow("SELECT count(*) FROM hostinfo WHERE hostname=$1 AND ipaddr=$2 "+
			"AND certfp!=$3 AND lastseen>$4", hostname, ipAddress, certfp, lastseen).Scan(&count)
		if err != nil {
			return "", err
		}
		if count > 0 {
			// Yes, the name is in use by another, newer host that has the same ip address
			// (and therefore DNS name). Leave it alone.
			return "", nil
		}
		// Is the result from DNS what the OS thinks its name is?
		if hostname == osHostname {
			// Then we must assume it is correct
			return hostname, nil
		}
		// The OS and DNS don't agree.
		// Well, is the name in use by any other machine at the moment?
		// One who's OS agrees with DNS?
		err = tx.QueryRow("SELECT count(*) FROM hostinfo WHERE "+
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
	err = tx.QueryRow("SELECT count(*) FROM ipranges WHERE $1 <<= iprange"+
		" AND NOT use_dns", ipAddress).Scan(&count)
	if err != nil {
		return "", err
	}
	if count > 0 {
		// Automatic naming without using DNS.
		// If the machine claims to have the hostname "donut.yourdomain.com",
		// the automatic name shouldn't conflict with other machines
		// with valid DNS records on the same domain.
		// Therefore, a suffix is added, so the name in Nivlheim becomes
		// "donut.yourdomain.com.local".
		// If another machine already has that name, the new name becomes
		// "donut.yourdomain.com.2.local".
		tx.QueryRow("SELECT count(*) FROM hostinfo WHERE hostname LIKE $1||'%.local'",
			osHostname).Scan(&count)
		if count > 0 {
			return fmt.Sprintf("%s.%d.local", osHostname, count+1), nil
		}
		return osHostname + ".local", nil
	}
	// Cannot be named automatically. Check if it has been manually approved.
	var hostname sql.NullString
	err = tx.QueryRow("SELECT hostname FROM waiting_for_approval WHERE ipaddr=$1 "+
		"AND approved", ipAddress).Scan(&hostname)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if !hostname.Valid {
		return "", nil
	}
	count = 1
	err = tx.QueryRow("SELECT count(*) FROM hostinfo WHERE hostname=$1",
		hostname.String).Scan(&count)
	if err != nil {
		return "", err
	}
	if count > 0 {
		// Send this machine back to approval, the hostname is already taken.
		tx.Exec("UPDATE waiting_for_approval SET approved=null WHERE ipaddr=$1",
			ipAddress)
		return "", nil
	}
	tx.Exec("DELETE FROM waiting_for_approval WHERE ipaddr=$1", ipAddress)
	return hostname.String, nil
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
