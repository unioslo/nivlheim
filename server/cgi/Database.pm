package Nivlheim::Database;
use strict;
use DBI;

sub connect_to_db() {
	# Read connection parameters from /etc/nivlheim/server.conf
	open(my $F, "/etc/nivlheim/server.conf");
	if (!$F) {
		print "Status: 500\r\nContent-Type: text/plain\r\n\r\n";
		print "Unable to read server.conf\n";
		exit;
	}
	my %dbparams = ();
	while (<$F>) {
		if (/^([pP[gG][a-zA-Z]+)=(.*)/) {
			$dbparams{uc $1} = $2;
		}
	}
	close($F);

	# Verify that server.conf contained the db config
	if (join('',values %dbparams) eq '') {
		print "Status: 500\r\nContent-Type: text/plain\r\n\r\n";
		print "server.conf is missing the database configuration options\n";
		exit;
	}

	# Connect to Postgres
	my %attr = ("AutoCommit" => 1);
	my $dbh = DBI->connect(
		sprintf("dbi:Pg:dbname=%s;host=%s;port=%d;",
			$dbparams{'PGDATABASE'}, $dbparams{'PGHOST'}, $dbparams{'PGPORT'}),
		$dbparams{'PGUSER'}, $dbparams{'PGPASSWORD'},
		\%attr);
	if (!$dbh) {
		print "Status: 500\r\nContent-Type: text/plain\r\n\r\n";
		print "Unable to connect to Postgres database:\n";
		print $DBI::errstr . "\n";
		exit;
	}
	return $dbh;
}

1; # Must return a value that evaluates to true
