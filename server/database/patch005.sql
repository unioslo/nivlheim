SET client_min_messages TO WARNING;

ALTER TABLE hostinfo ADD COLUMN ownergroup text;
ALTER TABLE apikeys RENAME COLUMN ownerid TO ownergroup;
ALTER TABLE apikeys ADD COLUMN groups text[],
	ADD COLUMN all_groups boolean not null default false,
	DROP COLUMN filter;

UPDATE db SET patchlevel = 5;
