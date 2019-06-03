package main

import (
	"os"
	"testing"

	"github.com/usit-gd/nivlheim/server/service/utility"
)

func TestTransaction(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}
	db := getDBconnForTesting(t)
	defer db.Close()

	utility.RunStatementsInTransaction(db, []string{
		"INSERT INTO files(filename) VALUES('1235')",
		"INSERT BLABLA THIS WILL ERROR",
	})

	var count int
	db.QueryRow("SELECT count(*) FROM files WHERE filename='1235'").Scan(&count)
	if count > 0 {
		t.Error("RunStatementsInTransaction doesn't rollback!")
	}
}

func TestBuildInsertStatement(t *testing.T) {
	sql, params := utility.BuildInsertStatement("mytable", map[string]interface{}{"foo": "bar", "num": 123})
	// the parameters may come in any order
	expectSQL1 := "INSERT INTO mytable(foo,num) VALUES($1,$2)"
	expectSQL2 := "INSERT INTO mytable(num,foo) VALUES($1,$2)"
	if sql != expectSQL1 && sql != expectSQL2 {
		t.Errorf("SQL: %s", sql)
	}
	if !((params[0] == "bar" && params[1] == 123) || (params[1] == "bar" && params[0] == 123)) {
		t.Errorf("Params are wrong: %v", params)
	}
}

func TestBuildUpdateStatement(t *testing.T) {
	sql, params := utility.BuildUpdateStatement("mytable", map[string]interface{}{"foo": "bar", "num": 123},
		"key", "zub")
	// the parameters may come in any order
	expectSQL1 := "UPDATE mytable SET foo=$1,num=$2 WHERE key=$3"
	expectSQL2 := "UPDATE mytable SET num=$1,foo=$2 WHERE key=$3"
	if sql != expectSQL1 && sql != expectSQL2 {
		t.Errorf("SQL: %s", sql)
	}
	if params[2] != "zub" {
		t.Errorf("Params are wrong: %v", params)
	} else if !((params[0] == "bar" && params[1] == 123) || (params[1] == "bar" && params[0] == 123)) {
		t.Errorf("Params are wrong: %v", params)
	}
}
