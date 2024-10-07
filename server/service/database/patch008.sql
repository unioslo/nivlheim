SET client_min_messages TO WARNING;

-- Create a sequence that will be used for the serial number field in client certificates
CREATE SEQUENCE cert_serial_seq START 1

UPDATE db SET patchlevel = 8;
