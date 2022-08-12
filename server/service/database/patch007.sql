SET client_min_messages TO WARNING;

-- Create an index because the certid field is referenced in the files table and slows down delete operations otherwise
CREATE INDEX IF NOT EXISTS files_originalcertid ON files(originalcertid);

UPDATE db SET patchlevel = 7;
