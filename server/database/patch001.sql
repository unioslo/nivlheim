SET client_min_messages TO WARNING;

DROP TABLE IF EXISTS waiting_for_approval, support, ipranges, hostinfo, files,
	certificates, tasks, settings, db CASCADE;

CREATE TABLE waiting_for_approval(
	approvalid serial PRIMARY KEY NOT NULL,
	ipaddr inet,
	hostname text,
	received timestamp with time zone,
	approved boolean
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
	fileid bigserial PRIMARY KEY NOT NULL,
	ipaddr inet,
	os_hostname text,
	certcn text,
	certfp text,
	filename text,
	received timestamp with time zone,
	mtime timestamp with time zone,
	content text,
	crc32 int4,
	is_command boolean not null default false,
	clientversion text,
	parsed boolean not null default false,
	current boolean not null default true,
	originalcertid int REFERENCES certificates(certid)
);

CREATE INDEX files_unparsed ON files(parsed) WHERE NOT parsed;
CREATE INDEX files_certfp_current ON files(certfp,current) WHERE current;
CREATE INDEX files_certfp_fname ON files(certfp,filename);

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
	ipaddr inet,
	certfp text PRIMARY KEY NOT NULL,
	lastseen timestamp with time zone,
	os text,
	os_edition text,
	os_family text,
	kernel text,
	manufacturer text,
	product text,
	serialno text,
	clientversion text,
	dnsttl timestamp with time zone
);

CREATE INDEX hostinfo_hostname ON hostinfo(hostname);
CREATE INDEX hostinfo_dnsttl ON hostinfo(dnsttl);

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

CREATE TABLE settings(
	key varchar(50) PRIMARY KEY NOT NULL,
	value text
);

CREATE TABLE db(
	patchlevel int NOT NULL
);
INSERT INTO db(patchlevel) VALUES(1);
