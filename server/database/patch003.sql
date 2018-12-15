SET client_min_messages TO WARNING;
DROP TABLE IF EXISTS apikeys,apikey_ips CASCADE;

CREATE TABLE apikeys(
	keyid varchar(32) PRIMARY KEY not null,
	ownerid text not null,
	comment text,
	expiry timestamp with time zone,
	readonly boolean not null default true,
	hostlistparams text
);

CREATE TABLE apikey_ips(
	keyid varchar(32) not null REFERENCES apikeys(keyid) ON UPDATE CASCADE ON DELETE CASCADE,
	iprange cidr not null
);

UPDATE db SET patchlevel = 3;