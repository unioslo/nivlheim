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
	parsed boolean
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
