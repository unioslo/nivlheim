SET client_min_messages TO WARNING;
DROP TABLE IF EXISTS apikeys,apikey_ips CASCADE;

CREATE TABLE apikeys(
	key varchar(32) PRIMARY KEY not null,
	ownerid text not null,
	comment text,
	created timestamp with time zone not null default now(),
	expires timestamp with time zone,
	readonly boolean not null default true,
	filter text
);

CREATE TABLE apikey_ips(
	key varchar(32) not null REFERENCES apikeys(key) ON UPDATE CASCADE ON DELETE CASCADE,
	iprange cidr not null
);

UPDATE db SET patchlevel = 3;