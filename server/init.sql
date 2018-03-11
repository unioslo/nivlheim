CREATE TABLE waiting_for_approval(
	approvalid serial PRIMARY KEY NOT NULL,
	ipaddr text,
	hostname text,
	received timestamp with time zone,
	approved boolean not null default false
);

CREATE TABLE certificates(
	certid serial PRIMARY KEY NOT NULL,
	issued timestamp with time zone NOT NULL,
	fingerprint text NOT NULL,
	commonname text NOT NULL,
	previous int,
	first int,
	revoked boolean not null default false,
	nonce int,
	cert text NOT NULL
);

CREATE INDEX cert_fingerprint ON certificates(fingerprint);

CREATE TABLE files(
	fileid serial PRIMARY KEY NOT NULL,
	ipaddr text,
	os_hostname text,
	certcn text,
	certfp text,
	filename text,
	received timestamp with time zone,
	mtime timestamp with time zone,
	content text,
	is_command boolean not null default false,
	clientversion text,
	parsed boolean not null default false,
	originalcertid int REFERENCES certificates(certid)
);

CREATE INDEX files_parsed ON files(parsed);

CREATE TABLE tasks(
	taskid serial PRIMARY KEY NOT NULL,
	url text not null unique,
	lasttry timestamp with time zone,
	status int not null default 0,
	delay int not null default 0,
	delay2 int not null default 0
);

CREATE TABLE hostinfo(
	hostname text UNIQUE,
	os_hostname text,
	ipaddr text,
	certfp text PRIMARY KEY NOT NULL,
	lastseen timestamp with time zone,
	os text,
	os_edition text,
	kernel text,
	vendor text,
	model text,
	serialno text,
	clientversion text,
	dnsttl timestamp with time zone
);

CREATE INDEX hostinfo_hostname ON hostinfo(hostname);

CREATE TABLE support(
	supportid serial PRIMARY KEY NOT NULL,
	serialno text NOT NULL,
	description text,
	start timestamp with time zone,
	expires timestamp with time zone,
	lastupdated timestamp with time zone
);

CREATE INDEX support_serial ON support(serialno);

CREATE TABLE ipranges(
	iprangeid serial PRIMARY KEY NOT NULL,
	iprange cidr NOT NULL,
	comment text,
	use_dns boolean not null default false
);
