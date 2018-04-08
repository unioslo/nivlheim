package main

// Create tasks to parse new files that have been read into the database
import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/lib/pq"
)

type parseFilesJob struct{}

var pfib *IntervalBuffer

func init() {
	RegisterJob(parseFilesJob{})
	pfib = NewIntervalBuffer(time.Minute)
}

func (s parseFilesJob) HowOften() time.Duration {
	return time.Second * 3
}

func (s parseFilesJob) Run(db *sql.DB) {
	rows, err := db.Query("SELECT fileid FROM files WHERE parsed = false")
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	concurrent := make(chan bool, 8)
	for rows.Next() {
		var fileid sql.NullInt64
		rows.Scan(&fileid)
		if fileid.Valid {
			concurrent <- true
			go func() {
				defer func() { <-concurrent }()
				parseFile(db, fileid.Int64)
				pfib.Add(1)
			}()
		}
	}
	// wait for running goroutines to finish
	for i := 0; i < cap(concurrent); i++ {
		concurrent <- true
	}
}

func parseFile(database *sql.DB, fileId int64) {
	tx, err := database.Begin()
	if err != nil {
		log.Println(err)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Println(r)
			tx.Rollback()
		} else if err != nil {
			log.Println(err)
			tx.Rollback()
		} else {
			tx.Exec("UPDATE files SET parsed = true WHERE fileid = $1", fileId)
			tx.Commit()
		}
	}()
	var filename, content, certcn, ipaddr, certfp, cVersion,
		osHostname sql.NullString
	var received pq.NullTime
	var isCommand sql.NullBool
	err = tx.QueryRow("SELECT filename, content, received, is_command, certcn,"+
		"ipaddr, certfp, clientversion, os_hostname FROM files WHERE fileid=$1",
		fileId).
		Scan(&filename, &content, &received, &isCommand, &certcn, &ipaddr,
			&certfp, &cVersion, &osHostname)
	if err != nil {
		return
	}
	if !certfp.Valid {
		panic(fmt.Sprintf("certfp is null for file %d", fileId))
	}
	// first, try to update as if there is an existing row
	result, err := tx.Exec("UPDATE hostinfo SET lastseen=$1,clientversion=$2 "+
		"WHERE certfp=$3", received, cVersion, certfp.String)
	if err != nil {
		return
	}
	rowcount, err := result.RowsAffected()
	if err != nil {
		return
	}
	if rowcount == 0 {
		// no existing row? then try to insert
		_, err = tx.Exec("INSERT INTO hostinfo(lastseen,ipaddr,clientversion,"+
			"os_hostname,certfp) VALUES($1,$2,$3,$4,$5)",
			received, ipaddr, cVersion, osHostname, certfp)
		if err != nil {
			// race condition (duplicate key value) or other error.
			return
		}
		triggerJob(handleDNSchangesJob{})
	} else {
		// The row exists already. This statement will set dnsttl to null
		// only if ipaddr or os_hostname changed.
		_, err = tx.Exec("UPDATE hostinfo SET ipaddr=$1, os_hostname=$2, "+
			"dnsttl=null WHERE (ipaddr!=$1 OR os_hostname!=$2) AND certfp=$3",
			ipaddr, osHostname, certfp)
		if err != nil {
			return
		}
	}

	if filename.String == "/etc/redhat-release" {
		var os, osEdition string
		rhel := regexp.MustCompile("^Red Hat Enterprise Linux (\\w+)" +
			".*(Tikanga|Santiago|Maipo)")
		m := rhel.FindStringSubmatch(content.String)
		if m != nil {
			osEdition = m[1]
			switch m[2] {
			case "Tikanga":
				os = "RHEL 5"
			case "Santiago":
				os = "RHEL 6"
			case "Maipo":
				os = "RHEL 7"
			}
		} else {
			fedora := regexp.MustCompile("^Fedora release (\\d+)")
			m = fedora.FindStringSubmatch(content.String)
			if m != nil {
				os = "Fedora " + m[1]
			} else {
				centos := regexp.MustCompile("^CentOS Linux release (\\d+)")
				m = centos.FindStringSubmatch(content.String)
				if m != nil {
					os = "CentOS " + m[1]
				}
			}
		}
		if os != "" && osEdition != "" {
			_, err = tx.Exec("UPDATE hostinfo SET os=$1, os_edition=$2 WHERE certfp=$3",
				os, osEdition, certfp.String)
		} else if os != "" {
			_, err = tx.Exec("UPDATE hostinfo SET os=$1 WHERE certfp=$2",
				os, certfp.String)
		}
		return
	}

	edition := regexp.MustCompile("/usr/lib/os.release.d/os-release-([a-z]+)")
	if m := edition.FindStringSubmatch(filename.String); m != nil {
		_, err = tx.Exec("UPDATE hostinfo SET os_edition=$1 WHERE certfp=$2",
			strings.Title(m[1]), certfp.String)
		return
	}

	if filename.String == "/usr/bin/dpkg-query -l" {
		ubuntuEdition := regexp.MustCompile("ubuntu-(desktop|server)")
		if m := ubuntuEdition.FindStringSubmatch(content.String); m != nil {
			_, err = tx.Exec("UPDATE hostinfo SET os_edition=$1 WHERE certfp=$2",
				strings.Title(m[1]), certfp.String)
		}
		return
	}

	if filename.String == "/etc/debian_version" {
		re := regexp.MustCompile("^(\\d+)\\.")
		if m := re.FindStringSubmatch(content.String); m != nil {
			_, err = tx.Exec("UPDATE hostinfo SET os=$1 WHERE certfp=$2",
				"Debian "+m[1], certfp.String)
		}
		return
	}

	if filename.String == "/etc/lsb-release" {
		re := regexp.MustCompile(`DISTRIB_ID=Ubuntu\nDISTRIB_RELEASE=(\d+)\.(\d+)`)
		if m := re.FindStringSubmatch(content.String); m != nil {
			_, err = tx.Exec("UPDATE hostinfo SET os=$1 WHERE certfp=$2",
				fmt.Sprintf("Ubuntu %s.%s", m[1], m[2]), certfp.String)
		}
		return
	}

	if filename.String == "/usr/bin/sw_vers" {
		re := regexp.MustCompile(`ProductName:\s+Mac OS X\nProductVersion:\s+(\d+\.\d+)`)
		if m := re.FindStringSubmatch(content.String); m != nil {
			_, err = tx.Exec("UPDATE hostinfo SET os=$1, os_edition=null "+
				"WHERE certfp=$2", "macOS "+m[1], certfp.String)
		}
		return
	}

	if filename.String == "(Get-WmiObject Win32_OperatingSystem).Caption" {
		reWinX := regexp.MustCompile(`Microsoft Windows (\d+)`)
		reWinServer := regexp.MustCompile(`Microsoft Windows Server (\d+)( R2)?`)
		if m := reWinX.FindStringSubmatch(content.String); m != nil {
			_, err = tx.Exec("UPDATE hostinfo SET os=$1, os_edition=null "+
				"WHERE certfp=$2", "Windows "+m[1], certfp.String)
		} else if m := reWinServer.FindStringSubmatch(content.String); m != nil {
			_, err = tx.Exec("UPDATE hostinfo SET os=$1, os_edition='Server' "+
				"WHERE certfp=$2", fmt.Sprintf("Windows %s%s", m[1], m[2]),
				certfp.String)
		}
		return
	}

	if filename.String == "/bin/uname -a" {
		re := regexp.MustCompile(`(\S+) \S+ (\S+)`)
		if m := re.FindStringSubmatch(content.String); m != nil {
			os := m[1]
			kernel := m[2]
			if os == "FreeBSD" {
				m = regexp.MustCompile(`^(\d+)`).FindStringSubmatch(kernel)
				if m != nil {
					os = "FreeBSD " + m[1]
				}
				_, err = tx.Exec("UPDATE hostinfo SET os=$1, os_edition=null, "+
					"kernel=$2 WHERE certfp=$3", os, kernel, certfp.String)
			} else {
				_, err = tx.Exec("UPDATE hostinfo SET kernel=$1 "+
					"WHERE certfp=$2", kernel, certfp.String)
			}
		}
		return
	}

	if filename.String == "/bin/uname -r" {
		kernel := strings.TrimSpace(strings.SplitN(content.String, "\n", 2)[0])
		_, err = tx.Exec("UPDATE hostinfo SET kernel=$1 "+
			"WHERE certfp=$2", kernel, certfp.String)
		return
	}

	if filename.String == "/usr/sbin/dmidecode -t system" {
		var vendor, model, serial sql.NullString
		if m := regexp.MustCompile(`Manufacturer: (.*)`).
			FindStringSubmatch(content.String); m != nil {
			vendor.String = strings.TrimSpace(m[1])
			vendor.String = strings.Title(strings.ToLower(vendor.String))
			vendor.Valid = len(vendor.String) > 0
		}
		if m := regexp.MustCompile(`Product Name: (.*)`).
			FindStringSubmatch(content.String); m != nil {
			model.String = strings.TrimSpace(m[1])
			model.String = strings.Title(strings.ToLower(model.String))
			model.Valid = len(model.String) > 0
		}
		if m := regexp.MustCompile(`Serial Number: (\w+)`).
			FindStringSubmatch(content.String); m != nil {
			serial.String = m[1]
			serial.Valid = len(serial.String) > 0
		}
		_, err = tx.Exec("UPDATE hostinfo SET vendor=$1,model=$2,serialno=$3"+
			"WHERE certfp=$4", vendor, model, serial, certfp.String)
		return
	}

	if filename.String == "/bin/freebsd-version -ku" {
		if m := regexp.MustCompile(`(\d+)\.(\d+)-RELEASE`).
			FindStringSubmatch(content.String); m != nil {
			_, err = tx.Exec("UPDATE hostinfo SET os=$1 WHERE certfp=$2",
				fmt.Sprintf("FreeBSD %s", m[1]), certfp.String)
		}
		return
	}

	if filename.String == "[System.Environment]::OSVersion|ConvertTo-Json" {
		//m := make(map[string]interface{})
		//err = json.Unmarshal(([]byte)content.String, m)
	}

	if filename.String == "Get-WmiObject Win32_computersystemproduct|Select Name,Vendor|ConvertTo-Json" {
	}
}
