SET client_min_messages TO WARNING;

DROP INDEX cert_fingerprint;
CREATE UNIQUE INDEX cert_fingerprint ON certificates(fingerprint);
ALTER TABLE certificates ADD CONSTRAINT unique_fingerprint UNIQUE USING INDEX cert_fingerprint;

DROP INDEX hostinfo_hostname; -- there's another index on hostname created by the UNIQUE constraint
ALTER TABLE hostinfo ADD COLUMN override_hostname text UNIQUE;

UPDATE db SET patchlevel = 4;