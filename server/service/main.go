package main

import (
	"bufio"
	"database/sql"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A Job is an internal piece of code that gets run periodically by this program
type Job interface {
	Run(db *sql.DB)
	HowOften() time.Duration
	//TODO change the HowOften func to a parameter for RegisterJob,
	//     and call it minimumTimeBetweenRuns or something similar.
}

type JobListElement struct {
	job               Job
	lastrun           time.Time
	lastExecutionTime time.Duration
	running, trigger  bool
	panicObject       interface{}
}

func RegisterJob(newjob Job) {
	jobs = append(jobs, JobListElement{job: newjob})
}

var jobs []JobListElement
var postgresSupportsOnConflict bool
var version string // should be set with -ldflags "-X main.version=1.2.3" during build

// Config (set in /etc/nivlheim/server.conf)
var oauth2ClientID string
var oauth2ClientSecret string
var oauth2Scopes []string
var oauth2AuthorizationEndpoint string
var oauth2TokenEndpoint string
var oauth2UserInfoEndpoint string
var oauth2LogoutEndpoint string
var authRequired bool
var authorizationPluginURL string
var devmode bool
var archiveDayLimit int = 30
var deleteDayLimit int = 180

func main() {
	log.SetFlags(0) // don't print a timestamp
	devmode = len(os.Args) >= 2 && os.Args[1] == "--dev"
	// in Go, the default random generator produces a deterministic sequence of values unless seeded
	rand.Seed(time.Now().UnixNano())

	// handle ctrl-c (SIGINT) and SIGTERM
	var quit bool
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		quit = true
		log.Println("\rShutting down...")
	}()
	defer log.Println("Stopped.")
	log.Println("Starting up.")

	if devmode {
		log.Println("Running in development mode.")
	}

	// Read config file
	readConfigFile()

	// Connect to database
	var dbConnectionString string
	if runtime.GOOS == "windows" {
		dbConnectionString = "sslmode=disable host=127.0.0.1 port=5432"
	} else {
		dbConnectionString = "host=/var/run/postgresql"
	}
	if !devmode {
		dbConnectionString += " dbname=apache"
	}
	db, err := sql.Open("postgres", dbConnectionString)
	if err != nil {
		log.Println(err)
		return
	}
	defer db.Close()

	// Determine capabilities of the database server
	postgresSupportsOnConflict = false
	var version sql.NullString
	err = db.QueryRow("select version()").Scan(&version)
	if err != nil {
		log.Println(err)
		return
	}
	if version.Valid {
		rePGVersion := regexp.MustCompile("PostgreSQL (\\d+.\\d+.\\d+)")
		mat := rePGVersion.FindStringSubmatch(version.String)
		if len(mat) >= 2 && len(mat[1]) > 0 {
			vstr := mat[1]
			log.Printf("PostgreSQL version: %s", vstr)
			postgresSupportsOnConflict = vstr >= "9.5"
		}
	}

	// Verify the schema patch level
	var patchlevel int
	const requirePatchLevel = 4
	db.QueryRow("SELECT patchlevel FROM db").Scan(&patchlevel)
	if patchlevel != requirePatchLevel {
		log.Printf("Error: Wrong database patch level. "+
			"Required: %d, Actual: %d\n", requirePatchLevel, patchlevel)
		return
	}

	go runAPI(db, 4040, devmode)
	go taskRunner(db, devmode)
	go loadContentForFastSearch(db)

	jobSlots := make(chan bool, 10) // max concurrent running jobs
	for !quit {
		// Run jobs
		for i, j := range jobs {
			if (time.Since(j.lastrun) > j.job.HowOften() || j.trigger) && !j.running {
				jobSlots <- true
				elem := &jobs[i]
				elem.running = true
				elem.lastrun = time.Now()
				elem.trigger = false
				go func() {
					defer func() {
						if r := recover(); r != nil {
							// if panicking, we want to recover, and keep the
							// object in elem.panicObject.
							elem.panicObject = r
						} else {
							// if NOT panicking, we want elem.panicObject to be nil.
							elem.panicObject = nil
						}
						elem.lastExecutionTime = time.Since(elem.lastrun)
						elem.lastrun = time.Now()
						elem.running = false
						<-jobSlots
					}()
					elem.job.Run(db)
				}()
			}
		}

		// Sleep
		time.Sleep(time.Second)
	}
	// wait for jobs to finish
	log.Println("Waiting for running jobs to finish...")
	left := cap(jobSlots)
	start := time.Now()
	for left > 0 && time.Since(start) <= time.Second*10 {
		select {
		case jobSlots <- true:
			left--
		default:
			time.Sleep(time.Second)
		}
	}
	if left > 0 {
		log.Printf("Terminating %d running jobs.", left)
	} else {
		log.Println("All jobs are finished.")
	}
}

func triggerJob(job Job) {
	for i, jobitem := range jobs {
		if reflect.TypeOf(jobitem.job) == reflect.TypeOf(job) {
			jobs[i].trigger = true
			return
		}
	}
	// If the job type isn't in the list, there's a programming error.
	// The type should have been registered by calling RegisterJob from an init() function.
	panic("Trying to trigger an unregistered job?")
}

func readConfigFile() {
	const configFileName = "/etc/nivlheim/server.conf"
	file, err := os.Open(configFileName)
	if err != nil {
		log.Printf("Unable to read %s: %v", configFileName, err)
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		keyAndValue := strings.SplitN(scanner.Text(), "=", 2)
		key, value := keyAndValue[0], keyAndValue[1]
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "oauth2clientid":
			oauth2ClientID = value
		case "oauth2clientsecret":
			oauth2ClientSecret = value
		case "oauth2scopes":
			oauth2Scopes = strings.Split(value, ",")
		case "oauth2authorizationendpoint":
			oauth2AuthorizationEndpoint = value
		case "oauth2tokenendpoint":
			oauth2TokenEndpoint = value
		case "oauth2userinfoendpoint":
			oauth2UserInfoEndpoint = value
		case "oauth2logoutendpoint":
			oauth2LogoutEndpoint = value
		case "authrequired":
			authRequired = isTrueish(value)
		case "authpluginurl":
			authorizationPluginURL = value
		case "archiveafterdays":
			archiveDayLimit, _ = strconv.Atoi(value)
		case "deleteafterdays":
			deleteDayLimit, _ = strconv.Atoi(value)
		}
	}
	if err = scanner.Err(); err != nil {
		log.Printf("Error while reading %s: %v", configFileName, err)
	}
	log.Printf("Read config file %s.", configFileName)
}
