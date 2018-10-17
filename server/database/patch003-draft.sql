SET client_min_messages TO WARNING;
DROP TABLE IF EXISTS apikeys,apikey_ips CASCADE;

CREATE TABLE apikeys(
    keyid varchar(32) PRIMARY KEY not null,
    owner_username text,
    comment text
);

CREATE TABLE apikey_ips(
    keyid varchar(32) REFERENCES apikeys(keyid) ON DELETE CASCADE,
    ipaddr inet
);

UPDATE db SET patchlevel = 3;