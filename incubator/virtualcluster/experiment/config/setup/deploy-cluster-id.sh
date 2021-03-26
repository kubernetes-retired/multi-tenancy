CLUSTER_ID=$1

if [[ -z $CLUSTER_ID ]]; then
    echo "cluster id cannot be empty"
    exit 1
fi

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"
YAML_TEMPLATE="$DIR/cluster-id.yaml.sed"

sed -e "s/CLUSTER_ID/$CLUSTER_ID/g" \
    "${YAML_TEMPLATE}"