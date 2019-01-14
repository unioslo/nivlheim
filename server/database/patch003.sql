SET client_min_messages TO WARNING;
DROP TABLE IF EXISTS apikeys,apikey_ips CASCADE;

CREATE TABLE apikeys(
	keyid serial PRIMARY KEY NOT NULL,
	key varchar(32) UNIQUE not null,
	ownerid text not null,
	comment text,
	created timestamp with time zone not null default now(),
	expires timestamp with time zone,
	readonly boolean not null default true,
	filter text
);

CREATE TABLE apikey_ips(
	keyid int not null REFERENCES apikeys(keyid) ON UPDATE CASCADE ON DELETE CASCADE,
	iprange cidr not null
);

UPDATE db SET patchlevel = 3;