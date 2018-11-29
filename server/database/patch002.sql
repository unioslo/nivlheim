SET client_min_messages TO WARNING;
DROP TABLE IF EXISTS customfields, hostinfo_customfields CASCADE;

CREATE TABLE customfields(
    fieldid serial PRIMARY KEY not null,
    name text UNIQUE not null,
    filename text,
    regexp text
);

CREATE TABLE hostinfo_customfields(
    certfp text not null REFERENCES hostinfo(certfp) ON UPDATE CASCADE ON DELETE CASCADE,
    fieldid int not null REFERENCES customfields(fieldid) ON UPDATE CASCADE ON DELETE CASCADE,
    value text,
    UNIQUE(certfp, fieldid)
);

UPDATE db SET patchlevel = 2;
