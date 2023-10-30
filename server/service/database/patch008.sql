SET client_min_messages TO WARNING;

-- Create a sequence for serial numbers for certificates
CREATE SEQUENCE cert_serial_seq;

UPDATE db SET patchlevel = 8;
