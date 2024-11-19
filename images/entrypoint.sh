#!/bin/sh

set -u -e -x

CNI_BIN_DIR=${CNI_BIN_DIR:-"/host/opt/cni/bin/"}

cp -f /bond $CNI_BIN_DIR

# Unless told otherwise, sleep forever.
# This prevents Kubernetes from restarting the pod repeatedly.
should_sleep=${SLEEP:-"true"}
echo "Done configuring CNI.  Sleep=$should_sleep"
while [ "$should_sleep" == "true"  ]; do
    sleep 1000000000000
done
