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
	expectSql := "INSERT INTO mytable(foo,num) VALUES($1,$2)"
	if sql != expectSql {
		t.Errorf("\n     Got %s\nExpected %s", sql, expectSql)
	}
	if params[0] != "bar" || params[1] != 123 {
		t.Errorf("Params are wrong: %v", params)
	}
}

func TestBuildUpdateStatement(t *testing.T) {
	sql, params := utility.BuildUpdateStatement("mytable", map[string]interface{}{"foo": "bar", "num": 123},
		"key", "zub")
	expectSql := "UPDATE mytable SET foo=$1,num=$2 WHERE key=$3"
	if sql != expectSql {
		t.Errorf("\n     Got %s\nExpected %s", sql, expectSql)
	}
	if params[0] != "bar" || params[1] != 123 || params[2] != "zub" {
		t.Errorf("Params are wrong: %v", params)
	}
}
