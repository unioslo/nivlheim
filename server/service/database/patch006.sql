SET client_min_messages TO WARNING;

ALTER TABLE certificates ADD trusted_by_cfengine boolean;

UPDATE db SET patchlevel = 6;
