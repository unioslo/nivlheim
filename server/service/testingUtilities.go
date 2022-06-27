package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"regexp"
	"testing"
)

// GetDBconnForTesting returns a database handle that points to a
// temporary tablespace that cleans up after the connection is closed.
// The function runs all the SQL scripts to create tables etc.
func getDBconnForTesting(t *testing.T) *sql.DB {
	// username
	user, err := user.Current()
	if err != nil {
		t.Fatalf(err.Error())
	}

	// defaults for testing
	var config = &Config{
		PGdatabase: user.Username,
		PGuser:     user.Username,
		PGpassword: "",
	}

	// Look for configuration overrides in the environment.
	UpdateConfigFromEnvironment(config)

	// Create a database connection
	dbConnectionString := fmt.Sprintf(
		"host=127.0.0.1 port=5432 dbname=%s user=%s password='%s' sslmode=disable",
		config.PGdatabase, config.PGuser, config.PGpassword)
	db, err := sql.Open("postgres", dbConnectionString)
	if err != nil {
		t.Fatal(err)
	}

	// Use a temporary tablespace that cleans up after the connection is closed
	_, err = db.Exec("SET search_path TO pg_temp")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}

	// It is important that the connection pool only uses this one connection,
	// because if it opens more, they won't have search_path set to pg_temp.
	db.SetMaxOpenConns(1)

	// Run the sql scripts that create all the tables
	for i := 1; i <= 999; i++ {
		sqlfile := fmt.Sprintf("patch%03d.sql", i)
		bytes, err := ioutil.ReadFile("database/" + sqlfile)
		if err != nil {
			_, ok := err.(*os.PathError)
			if ok {
				break
			}
			db.Close()
			t.Fatalf("Couldn't read %s", sqlfile)
		}
		_, err = db.Exec(StripProceduresAndTriggers(string(bytes)))
		if err != nil {
			db.Close()
			t.Fatalf("%s: %v", sqlfile, err)
		}
	}

	return db
}

// StripProceduresAndTriggers removes SQL statements that create
// stored procedures and triggers.
// When using pg_temp, you can't use the exact same syntax for creating
// stored procedures as normal, so regular database scripts won't work.
func StripProceduresAndTriggers(script string) string {
	re := regexp.MustCompile("--start_of_procedures\n(?s:.+?)--end_of_procedures\n")
	for n := 1; n < 100; n++ {
		m := re.FindStringIndex(script)
		if m == nil {
			break
		}
		script = script[0:m[0]] + script[m[1]:]
	}
	// also change "create unlogged table" to "create table", since unlogged
	// tables can't be created in a temporary tablespace.
	script = regexp.MustCompile(`(?i)CREATE UNLOGGED TABLE`).
		ReplaceAllString(script, "CREATE TABLE")

	// Can't use extension pg_trgm during testing, it might not be available
	re = regexp.MustCompile(`(?i)CREATE INDEX \w+ ON \w+ USING gin\(\w+ gin_trgm_ops\);`)
	script = re.ReplaceAllString(script, "")

	return script
}
