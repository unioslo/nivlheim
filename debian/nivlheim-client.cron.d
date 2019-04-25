#
# Regular cron jobs for the nivlheim-client package
#
*/5 * * * *  root  [ -x /usr/sbin/nivlheim_client ] && /usr/sbin/nivlheim_client -minperiod 3600 -sleeprandom 300 > /dev/null
