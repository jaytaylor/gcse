#!/usr/bin/env bash

cd "$(dirname "$0")/.."

while [ true ] ; do
    gcse-crawler 2>&1 | tee crawl.log.$(date +%s)
    sleep 60
done
