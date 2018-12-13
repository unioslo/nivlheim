SET client_min_messages TO WARNING;
DROP TABLE IF EXISTS apikeys,apikey_ips CASCADE;

CREATE TABLE apikeys(
	keyid varchar(32) PRIMARY KEY not null,
	ownerid text,
	comment text,
	expiry timestamp with time zone,
	readonly boolean not null default true,
	hostlistparams text
);

CREATE TABLE apikey_ips(
	keyid varchar(32) REFERENCES apikeys(keyid) ON UPDATE CASCADE ON DELETE CASCADE,
	ipaddr inet
);

UPDATE db SET patchlevel = 3;