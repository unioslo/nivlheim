#
# Regular cron jobs for the nivlheim-client package
#
0 4	* * *	root	[ -x /usr/sbin/nivlheim_client ] && /usr/sbin/nivlheim_client
