package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"regexp"
	"testing"
)

func getDBconnForTesting(t *testing.T) *sql.DB {
	// Create a database connection
	db, err := sql.Open("postgres", "sslmode=disable host=/var/run/postgresql")
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
	// Run the sql script that creates all the tables
	bytes, err := ioutil.ReadFile("../init.sql")
	if err != nil {
		db.Close()
		t.Fatal("Couldn't read init.sql")
	}
	_, err = db.Exec(StripProceduresAndTriggers(string(bytes)))
	if err != nil {
		db.Close()
		t.Fatalf("init.sql: %v", err)
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
	return script
}

// IsEqualJSON returns true if the 2 supplied strings contain JSON data
// that is semantically equal.
func IsEqualJSON(s1, s2 string) (bool, error) {
	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal([]byte(s1), &o1)
	if err != nil {
		return false, fmt.Errorf("Error unmarshalling string 1 :: %s", err.Error())
	}
	err = json.Unmarshal([]byte(s2), &o2)
	if err != nil {
		return false, fmt.Errorf("Error unmarshalling string 2 :: %s", err.Error())
	}

	return reflect.DeepEqual(o1, o2), nil
}
