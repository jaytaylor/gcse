#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

if [ $(id -u) -ne 0 ] ; then
    echo 'error: must be run as root' 1>&2
    exit 1
fi

set -x

origUser="$(logname)"

cd "$(dirname "$0")/.."

mkdir -p /mnt/ramdisk

while [ -n "$(mount | grep 'on \/mnt\/ramdisk' || :)" ] ; do
    umount /mnt/ramdisk
done

mount -t tmpfs -o size=512m tmpfs /mnt/ramdisk

if [ -L data ] ; then
    unlink data
fi

if [ -e data ] ; then
    dataBak="data.bak.$(date +%s)"
    echo "INFO: Moving existing data dirrectory to ${dataBak}" 1>&2
    mv data "${dataBak}"
fi

ln -s /mnt/ramdisk "$(pwd)/data"

echo "INFO: Starting gcse-tocrawl" 1>&2
export PATH="${PATH}:/home/${origUser}/go/bin"
gcse-tocrawl
echo "INFO: Finished gcse-tocrawl" 1>&2

echo -n 'INFO: Copying ramdisk data to stable storage ..' 1>&2
newData="data.$(date +%s)"
mkdir -p "${newData}"
cp -a data/* "${newData}/"
chown -R "${origUser}:${origUser}" data/
echo 'OK'

echo -n 'INFO: Cleaning up ramdisk ..' 1>&2
unlink data
umount /mnt/ramdisk
mv "${newData}" data
echo 'OK'

