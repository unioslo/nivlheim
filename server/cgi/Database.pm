package Nivlheim::Database;
use strict;
use DBI;

sub connect_to_db() {
	my %dbparams = ();
	$dbparams{'PGPORT'} = 5432; # default

	# Try to read connection parameters from /etc/nivlheim/server.conf
	open(my $F, "/etc/nivlheim/server.conf");
	if ($F) {
		while (<$F>) {
			if (/^([pP[gG][a-zA-Z]+)=(.*)/) {
				$dbparams{uc $1} = $2;
			}
		}
		close($F);
	}

	# Also try to get connection parameters from the environment,
	# this will override server.conf settings
	$_ = $ENV{'NIVLHEIM_PGDATABASE'}; $dbparams{'PGDATABASE'} = $_ if $_;
	$_ = $ENV{'NIVLHEIM_PGHOST'}; $dbparams{'PGHOST'} = $_ if $_;
	$_ = $ENV{'NIVLHEIM_PGPORT'}; $dbparams{'PGPORT'} = $_ if $_;
	$_ = $ENV{'NIVLHEIM_PGUSER'}; $dbparams{'PGUSER'} = $_ if $_;
	$_ = $ENV{'NIVLHEIM_PGPASSWORD'}; $dbparams{'PGPASSWORD'} = $_ if $_;

	# Error if we didn't get the database config
	if (join('',values %dbparams) eq '') {
		print "Status: 500\r\nContent-Type: text/plain\r\n\r\n";
		print "Database connection info not found in server.conf or environment\n";
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
