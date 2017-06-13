CREATE TABLE IF NOT EXISTS waiting_for_approval(
	ipaddr varchar(100),
	hostname varchar(100),
	received timestamp with time zone,
	approved boolean
);
