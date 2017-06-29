CREATE TABLE IF NOT EXISTS waiting_for_approval(
	ipaddr text,
	hostname text,
	received timestamp with time zone,
	approved boolean
);

CREATE TABLE IF NOT EXISTS files(
	fileid serial,
	ipaddr text,
	clienthostname text,
	certcn text,
	certfp text,
	filename text,
	received timestamp with time zone,
	content text,
	is_command boolean,
	clientversion text,
	parsed boolean not null default false
);

CREATE INDEX files_parsed ON files(parsed);

CREATE TABLE IF NOT EXISTS jobs(
	jobid serial,
	url text not null unique,
	lasttry timestamp with time zone,
	status int not null default 0,
	delay int not null default 0,
	delay2 int not null default 0
);

CREATE TABLE IF NOT EXISTS hostinfo(
	hostname text,
	ipaddr text,
	certfp text PRIMARY KEY NOT NULL,
	kernel text,
	type text,
	lastseen timestamp with time zone,
	os text,
	osclass text,
	vendor text,
	model text,
	serialno text,
	nivlheim_version text
);

CREATE TABLE IF NOT EXISTS warranty(
	serialno text NOT NULL,
	description text,
	start timestamp with time zone,
	expires timestamp with time zone,
	lastupdated timestamp with time zone
);

CREATE TABLE IF NOT EXISTS api_error(
	serialno text NOT NULL,
	http_status int,
	api_status int,
	api_message text,
	ts timestamp with time zone
);
