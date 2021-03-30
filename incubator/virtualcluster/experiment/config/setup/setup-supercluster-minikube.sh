#!/bin/bash

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"

CLUSTER_ID=$1
if [[ -z $CLUSTER_ID ]]; then
    echo "cluster id cannot be empty"
    exit 1
fi

log() {
    now=$(date "+%F %T")
    cat <<EOF
${now} $*
EOF
}

WORKDIR="$DIR/cluster-$CLUSTER_ID"
KUBECONFIG_PATH="$WORKDIR/kubeconfig"
KUBECONFIG_SECRET_NAME="cluster-${CLUSTER_ID}-config"
KUBECONFIG_SECRET_PATH_SYNCER="$WORKDIR/secret-for-vc-syncer.yaml"
KUBECONFIG_SECRET_PATH_SCHEDULER="$WORKDIR/secret-for-scheduler.yaml"
CLUSTER_ID_YAML_PATH="$WORKDIR/cluster-id.yaml"
SYNCER_YAML_PATH="$WORKDIR/vc-syncer.yaml"
CLUSTER_CR_YAML_PATH="$WORKDIR/cluster-cr.yaml"
log "create workdir $WORKDIR"
mkdir -p $WORKDIR >/dev/null
pushd $WORKDIR >/dev/null

log "generate kubeconfig file $KUBECONFIG_PATH"
cat > $KUBECONFIG_PATH << EOL
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: $(cat $HOME/.minikube/ca.crt |base64)
    server: https://$(minikube ip -p ${CLUSTER_ID}):8443
  name: ${CLUSTER_ID}
contexts:
- context:
    cluster: ${CLUSTER_ID}
    namespace: default
    user: ${CLUSTER_ID}
  name: ${CLUSTER_ID}
current-context: ${CLUSTER_ID}
kind: Config
preferences: {}
users:
- name: ${CLUSTER_ID}
  user:
    client-certificate-data: $(cat $HOME/.minikube/profiles/$CLUSTER_ID/client.crt |base64)
    client-key-data: $(cat $HOME/.minikube/profiles/$CLUSTER_ID/client.key |base64)
EOL

log "generate vc-syncer kubeconfig secret"
kubectl create secret generic --dry-run=client -n vc-manager ${KUBECONFIG_SECRET_NAME} --from-file=config=${KUBECONFIG_PATH} -oyaml > ${KUBECONFIG_SECRET_PATH_SYNCER}

log "generate vc-scheduler kubeconfig secret"
kubectl create secret generic --dry-run=client ${CLUSTER_ID} --from-file=admin-kubeconfig=${KUBECONFIG_PATH} -oyaml > ${KUBECONFIG_SECRET_PATH_SCHEDULER}

log "generate vc syncer deployment yaml"
${DIR}/deploy-syncer.sh -s vc-syncer-$CLUSTER_ID -c ${KUBECONFIG_SECRET_NAME} >${SYNCER_YAML_PATH} 2>&1

log "generate cluster id yaml"
${DIR}/deploy-cluster-id.sh $CLUSTER_ID >${CLUSTER_ID_YAML_PATH} 2>&1

log "generate fake cluster cr yaml"
cat > $CLUSTER_CR_YAML_PATH << EOL
apiVersion: cluster.x-k8s.io/v1alpha4
kind: Cluster
metadata:
  name: ${CLUSTER_ID}
  namespace: default
status:
  phase: Provisioned
EOL
