#
# Regular cron jobs for the nivlheim-client package
#
*/5 * * * *  root  [ -x /usr/bin/nivlheim_client ] && /usr/bin/nivlheim_client -minperiod 3600 -sleeprandom 300 > /dev/null
