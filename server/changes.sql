ALTER TABLE files ADD COLUMN current boolean NOT NULL DEFAULT false;
DROP INDEX files_parsed;
CREATE INDEX files_unparsed ON files(parsed) WHERE NOT parsed;
CREATE INDEX files_certfp_current ON files(certfp,current) WHERE current;
ALTER TABLE files ALTER COLUMN current SET DEFAULT true;
UPDATE files t1 SET current=true FROM
	(SELECT certfp,filename,max(received) AS maxtime FROM files 
	GROUP BY certfp,filename) AS t2 
	WHERE t1.certfp=t2.certfp AND t1.filename=t2.filename 
	AND t1.received=t2.maxtime;
