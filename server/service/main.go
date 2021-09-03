package main

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"syscall"
	"time"
	"github.com/unioslo/nivlheim/server/service/utility"
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
var config = &Config{}
var devmode bool

// Embed the database patches for schema migration.
//go:embed database/*.sql
var databasePatches embed.FS
func migrateDatabase(db *sql.DB, currentPatchLevel int, targetPatchLevel int) (err error) {
	if currentPatchLevel > targetPatchLevel {
		return errors.New("I'm too old for this.")
	} else {
		log.Println("Running migrations.")
	}
	patchStatements := []string{}
	for i := currentPatchLevel + 1; i <= targetPatchLevel; i++ {
		patchName := fmt.Sprintf("database/patch%03d.sql", i)
		log.Printf("Applying database patch %s...", patchName)
		patch, err := databasePatches.ReadFile(patchName)
		if err != nil {
			return err
		} else {
			patchStatements = append(patchStatements, string(patch[:]))
		}
	}
	err = utility.RunStatementsInTransaction(db, patchStatements)
	return err
}

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
	const configFileName = "/etc/nivlheim/server.conf"
	var err error
	err = UpdateConfigFromFile(config, configFileName)
	if err != nil {
		log.Printf("Unable to read %s: %v", configFileName, err)
		return
	}
	log.Printf("Read config file %s.", configFileName)

	// Look for configuration overrides in the environment.
	UpdateConfigFromEnvironment(config)

	// Connect to database
	var dbConnectionString string
	if config.PGhost != "" {
		if config.PGport == 0 {
			config.PGport = 5432
		}
		dbConnectionString = fmt.Sprintf(
			"host=%s port=%d dbname=%s user=%s password='%s' sslmode=%s",
			config.PGhost, config.PGport, config.PGdatabase,
			config.PGuser, config.PGpassword, config.PGsslmode)
		log.Printf("Connecting to database %s on host %s\n",
			config.PGdatabase, config.PGhost)
	} else {
		log.Println("Missing database connection parameters")
		return
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
		rePGVersion := regexp.MustCompile("PostgreSQL (\\d+\\.\\d+)")
		mat := rePGVersion.FindStringSubmatch(version.String)
		if len(mat) >= 2 && len(mat[1]) > 0 {
			vstr := mat[1]
			log.Printf("PostgreSQL version: %s", vstr)
			postgresSupportsOnConflict = vstr >= "9.5"
		}
	}

	// Verify the schema patch level
	var patchLevel int
	const requirePatchLevel = 6
	err = db.QueryRow("SELECT patchlevel FROM db").Scan(&patchLevel)
	if err != nil {
		patchLevel = 0
	}
	if patchLevel != requirePatchLevel {
		log.Printf("Database patch level is %d, expected %d.",
			patchLevel, requirePatchLevel)
		err := migrateDatabase(db, patchLevel, requirePatchLevel)
		if err != nil {
			log.Fatal(err)
		}
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
