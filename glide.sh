#!/usr/bin/env bash

# Copyright (c) 2017 Intel Corp
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
# implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
