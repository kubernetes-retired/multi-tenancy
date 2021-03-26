#!/bin/bash


show_help () {
cat << USAGE
usage: $0 [ -s SYNCER_NAME ] [ -c SUPER_CLUSTER_CONFIG ] [ -t YAML-TEMPLATE ]
    -s : the vc-syncer deployment name.
    -c : the super cluster configmap name.
USAGE
exit 0
}

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"
YAML_TEMPLATE="$DIR/syncer.yaml.sed"

while getopts "hs:c:" opt; do
    case "$opt" in
    h)  show_help
        ;;
    s)  SYNCER_NAME="$OPTARG"
        ;;
    c)  SUPER_CLUSTER_CONFIG=$OPTARG
        ;;
    t)  YAML_TEMPLATE=$OPTARG
        ;;
    esac
done

if [[ -z $SYNCER_NAME ]]; then
    echo "vc-syncer name cannot be empty"
    show_help
    exit 1
fi

if [[ -z $SUPER_CLUSTER_CONFIG ]]; then
    echo "super cluster config cannot be empty"
    show_help
    exit 1
fi

sed -e "s/SYNCER_NAME/$SYNCER_NAME/g" \
    -e "s/SUPER_CLUSTER_CONFIG/$SUPER_CLUSTER_CONFIG/g" \
    "${YAML_TEMPLATE}"
