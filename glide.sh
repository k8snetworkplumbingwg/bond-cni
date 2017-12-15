#!/usr/bin/env bash
set -e

## resolve dependencies with Glide
command -v glide >/dev/null 2>&1
if [ $? -ne 0 ] ; then
    echo "Installing Glide..."
    curl https://glide.sh/get | sh
fi

if [ ! -e ./vendor ]; then
    if [ ! -e ./glide.yaml ]; then
        glide create
    fi
    glide --quiet install
elif [ -e glide.lock ] && [ -e ./glide.yaml ]; then
    glide up --strip-vendor
else
    glide create && glide install
fi
