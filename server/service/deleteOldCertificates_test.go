package main

import (
	"os"
	"testing"

	"github.com/unioslo/nivlheim/server/service/utility"
)

func TestDeleteOldCertificates(t *testing.T) {
	if os.Getenv("NOPOSTGRES") != "" {
		t.Log("No Postgres, skipping test")
		return
	}

	// Create a database connection
	db := getDBconnForTesting(t)
	defer db.Close()

	// Add some data for testing
	err := utility.RunStatementsInTransaction(db, []string{
		// Expired certificate, should be deleted
		"INSERT INTO certificates(certid, first, previous, fingerprint, cert, issued, commonname) VALUES(1,1,NULL,'abcd','Not After : Jan 01 13:14:15 2020 GMT',now(),'foo')",
		// Expired certificate, but referenced by another certificate, should be kept
		"INSERT INTO certificates(certid, first, previous, fingerprint, cert, issued, commonname) VALUES(2,2,NULL,'bcde','Not After : Jan 01 13:14:15 2020 GMT',now(),'foo')",
		// Expired certificate, but still referenced by a file, should be kept
		"INSERT INTO certificates(certid, first, previous, fingerprint, cert, issued, commonname) VALUES(3,2,2,'cdef','Not After : Jan 01 13:14:15 2021 GMT', now(), 'foo')",
		"INSERT INTO files(originalcertid,certfp) VALUES(2,'cdef')",
		// Certificate that has a valid date, should be kept
		"INSERT INTO certificates(certid, first, previous, fingerprint, cert, issued, commonname) VALUES(4,4,NULL,'efefef','Not After : Jan 01 13:14:15 2070 GMT',now(),'bar')",
		// Certificate that is referenced by a hostinfo row and no files, should never happen but if it does we've covered that too
		"INSERT INTO certificates(certid, first, previous, fingerprint, cert, issued, commonname) VALUES(5,5,NULL,'dedede','Not After : Jan 01 13:14:15 2021 GMT', now(), 'baz')",
		"INSERT INTO hostinfo(hostname,certfp) VALUES('baz','dedede')",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Run the function
	job := deleteOldCertificatesJob{}
	job.Run(db)

	// Look at the result
	list, err := QueryColumn(db, "SELECT certid FROM certificates ORDER BY certid")
	if err != nil {
		t.Fatal(err)
	}
	correctResult := []int64{2, 3, 4, 5}
	if len(list) != len(correctResult) {
		t.Fatalf("Expected %v, got %v", correctResult, list)
	}
	for i, v := range list {
		n := v.(int64)
		if n != correctResult[i] {
			t.Fatalf("Expected %v, got %v", correctResult, list)
		}
	}
}
