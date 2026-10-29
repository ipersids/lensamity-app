#!/bin/sh
/usr/local/bin/api &
exec nginx -g 'daemon off;'
