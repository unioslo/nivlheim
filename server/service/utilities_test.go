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
