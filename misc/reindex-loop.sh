#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

if [ $(id -u) -ne 0 ] ; then
    echo 'error: must be run as root' 1>&2
    exit 1
fi

while [ true ] ; do
    echo date
    gcse-mergedocs
    gcse-indexer
    chown -R ppx:ppx .git .
    service gcse-service-web restart
    sleep 600
done

