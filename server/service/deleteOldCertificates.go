package main

import (
	"database/sql"
	"log"
	"time"

	"github.com/unioslo/nivlheim/server/service/utility"
)

type deleteOldCertificatesJob struct{}

func init() {
	RegisterJob(deleteOldCertificatesJob{})
}

func (p deleteOldCertificatesJob) HowOften() time.Duration {
	return time.Hour * 24
}

func (p deleteOldCertificatesJob) Run(db *sql.DB) {
	// Delete old certificates from the database table
	// Criteria:
	// - expired (past the "not after" date)
	// - not referenced from any files/hostinfo rows
	// - not referenced by any other certificate in the table (through the "previous" column) that was in turn referenced by files/hostinfo
	err := utility.RunStatementsInTransaction(db, []string{
		// First, find certificates in use, create a temporary table
		`SELECT * INTO TEMP TABLE certs_in_use FROM (
			WITH RECURSIVE previouscerts AS (
				SELECT certid,previous FROM certificates WHERE fingerprint IN (SELECT distinct certfp FROM files UNION SELECT distinct certfp FROM hostinfo)
				UNION
				SELECT certid,previous FROM certificates c, files f WHERE c.certid=f.originalcertid
				UNION
				SELECT DISTINCT c.certid,c.previous FROM certificates c,previouscerts pc WHERE c.certid=pc.previous
			) SELECT * FROM previouscerts
		) AS foo`,
		// Then, find certificates that fit the criteria to be deleted
		`SELECT c.certid INTO TEMP TABLE certs_to_be_deleted
		FROM certificates c
		LEFT JOIN certs_in_use use ON c.certid=use.certid
		WHERE to_timestamp( (regexp_match(cert,'Not After : (.* GMT)'))[1], 'Mon DD HH24:MI:SS YYYY') < now() AND use.certid IS NULL`,
		// Delete the certificates
		`DELETE FROM certificates WHERE certid IN (SELECT certid FROM certs_to_be_deleted)`,
		// Last, drop the temp tables and index
		`DROP TABLE certs_in_use, certs_to_be_deleted`,
	})
	if err != nil {
		log.Panic(err)
	}
}
