CREATE TABLE IF NOT EXISTS waiting_for_approval(
	ipaddr text,
	hostname text,
	received timestamp with time zone,
	approved boolean
);

CREATE TABLE IF NOT EXISTS files(
	id serial,
	ipaddr text,
	clienthostname text,
	certcn text,
	certfp text,
	filename text,
	received timestamp with time zone,
	content text,
	is_command boolean,
	clientversion text
);

CREATE TABLE IF NOT EXISTS jobs(
	filename text,
	lasttry timestamp with time zone,
	status int,
	delay int,
	delay2 int
);
