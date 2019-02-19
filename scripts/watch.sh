#!/bin/bash

# Copyright 2019 Tuxera Oy. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

function main {

    for c in $(aws ecs list-clusters --query 'clusterArns[*]' --output text); do
        echo "----------- cluster: $c ---------------------"
        for s in $(aws ecs list-services --cluster $c --query 'serviceArns[*]' --output text); do
            echo "---------- service: $s ------------------"
        done
    done
}

main $*
