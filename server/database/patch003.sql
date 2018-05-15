CREATE UNLOGGED TABLE tasks2(
	taskid serial PRIMARY KEY NOT NULL,
	url text not null unique,
	lasttry timestamp with time zone,
	status int not null default 0,
	delay int not null default 0,
	delay2 int not null default 0
);
INSERT INTO tasks2(url,lasttry,status,delay,delay2)
	SELECT url,lasttry,status,delay,delay2 FROM tasks;
DROP TABLE tasks;
ALTER TABLE tasks2 RENAME TO tasks;
ALTER INDEX tasks2_pkey RENAME TO tasks_pkey;
ALTER INDEX tasks2_url_key RENAME TO tasks_url_key;

--start_of_procedures
DROP TRIGGER files_update_tsvec ON files;
DROP FUNCTION upd_tsvec();
--end_of_procedures
DROP INDEX files_tsvec;
ALTER TABLE files DROP COLUMN tsvec;

CREATE INDEX files_content_trgm ON files USING gin(content gin_trgm_ops);

UPDATE db SET patchlevel=3;
