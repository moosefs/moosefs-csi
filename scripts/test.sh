#!/bin/bash

OUTPUT_FILE="/tmp/moosefs-csi.cred.yaml"

function main {

    curdir=$(dirname $0)
    if [[ $curdir == "." ]]; then
        curdir=$(pwd)
    fi

    check_kubectl

    refresh_sts $curdir

    apply_csi $curdir
}

function check_kubectl {
    kubectl get po &>/dev/null
    if [[ $? -ne 0 ]]; then
        echo "ERROR: No kubernetes running, cannot proceed"
        exit -1
    fi
}

function refresh_sts {
    # order of array: secret, sess-tok, exp, Id
    cred=($(aws sts get-session-token --duration-seconds 21600 --query 'Credentials.*' --output text))
    sed -e s~\"AWS_SECRET\"~\"${cred[0]}\"~g \
        -e s~\"AWS_SESSION_TOKEN\"~\"${cred[1]}\"~g \
        -e s~\"AWS_ACCESS_KEY_ID\"~\"${cred[3]}\"~g \
        $1/../deploy/kubernetes/moosefs-csi.yaml > $OUTPUT_FILE 
    echo "INFO: Created file: $OUTPUT_FILE with valid credentials"
}

function apply_csi {
    # kubectl apply 
    kubectl apply -f $OUTPUT_FILE
    echo "INFO: Applied $OUTPUT_FILE to k8s"
}

main $*
