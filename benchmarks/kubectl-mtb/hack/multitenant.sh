#!/bin/bash
set -e
set -o pipefail

# Add user to k8s using service account, no RBAC (must create RBAC after this script)
if [[ -z "$1" ]]; then
    echo "usage: $0 <number of tenants>"
    exit 1
fi

COUNT=$1
TARGET_FOLDER="/tmp/kube"
TENANT_CRD="https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenant.yaml"
TENANT_NAMESPACE_CRD="https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenantnamespace.yaml"
TENANT_CONTROLLER_MANAGER="https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/manager/all_in_one.yaml"
TENANT_SAMPLE="https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/samples/tenancy_v1alpha1_tenant.yaml"
TENANT_NAMESPACE_SAMPLE="https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/samples/tenancy_v1alpha1_tenantnamespace.yaml"

# Create tenant CRD
create_tenant_crd() {
    echo -e "\\nCreating tenant CRD from ${TENANT_CRD}"
    kubectl apply -f "${TENANT_CRD}"
}

# Create tenant namespace CRD
create_tenantnamespace_crd() {
    FILE="/tmp/kube/TENANT_NAMESPACE_CRD.yaml"
    if test -f "$FILE"; then
        kubectl apply -f "$FILE"
        return
    fi
    wget -O "${FILE}" "${TENANT_NAMESPACE_CRD}"
    echo -e "\\nCreating tenantnamespace CRD from ${TENANT_NAMESPACE_CRD}"
    kubectl apply -f "${FILE}"
}

# Create Tenant Controller Manager
create_tenant_controller_manager() {
    FILE="/tmp/kube/TENANT_CONTROLLER_MANAGER.yaml"
    if test -f "$FILE"; then
        kubectl apply -f "$FILE"
        return
    fi
    wget -O "${FILE}" "${TENANT_CONTROLLER_MANAGER}"
    echo -e "\\nCreating tenant controller manager from ${TENANT_CONTROLLER_MANAGER}"
    kubectl apply -f "${FILE}"
}

# Create Tenants Using tenant-CRD 
create_tenant_sample() {
    NAME=$1
    echo -e "\\nCreating tenant-${NAME}"
    FILE="/tmp/kube/TENANT-${NAME}.yaml"
    if test -f "$FILE"; then
        sed -ri "s/sample/${NAME}-cr/; s/tenant1admin/tenant${NAME}admin/" "${FILE}"
        kubectl apply -f "$FILE"
        return
    fi
    wget -O "${FILE}" "${TENANT_SAMPLE}"
    sed -ri "s/sample/${NAME}-cr/; s/tenant1admin/tenant${NAME}admin/" "${FILE}"
    kubectl apply -f "${FILE}"
}

# Create Temporary folder to contain all yaml and certi
create_target_folder() {
    echo -n "Creating target directory to hold files in ${TARGET_FOLDER}"
    mkdir -p "${TARGET_FOLDER}"
}

# Create Service account for the Tenant-admin
create_service_account() {
    NAME=$1
    ADMIN_NAMESPACE_CREATED=0
    echo -e "\\nWaiting for tenant${NAME}admin namespace to be created"
    while [ "$ADMIN_NAMESPACE_CREATED" != 1 ]; do
        if kubectl get ns | grep -q "tenant${NAME}admin"; then
            ADMIN_NAMESPACE_CREATED=1
            break
        fi
        sleep 7
    done
    echo -e -n "Enter service account name for sa of tenant-${NAME}-cr.."
    SERVICE_ACCOUNT_NAME="t${NAME}-admin${NAME}"
    KUBECFG_FILE_NAME="/tmp/kube/k8s-${SERVICE_ACCOUNT_NAME}-tenant${NAME}admin-conf"
    echo -e "\\nCreating a service account in tenant${NAME}admin namespace: ${SERVICE_ACCOUNT_NAME}"
    kubectl create sa "${SERVICE_ACCOUNT_NAME}" --namespace "tenant${NAME}admin"
}

get_secret_name_from_service_account() {
    echo -e "\\nGetting secret of service account ${SERVICE_ACCOUNT_NAME} on tenant${NAME}admin"
    NAME=$1
    SA=0
    echo -e "\\nWaiting for SA to be created"
    while [ "$SA" != 1 ]; do
        if kubectl get sa "${SERVICE_ACCOUNT_NAME}" --namespace="tenant${NAME}admin" | grep -q "${SERVICE_ACCOUNT_NAME}"; then
            SA=1
            break
        fi
        sleep 7
    done
    SECRET_NAME=$(kubectl get sa "${SERVICE_ACCOUNT_NAME}" --namespace="tenant${NAME}admin" -o json | jq -r .secrets[].name)
    echo "Secret name: ${SECRET_NAME}"
}

extract_ca_crt_from_secret() {
    echo -e -n "\\nExtracting ca.crt from secret"
    kubectl get secret --namespace "tenant${NAME}admin" "${SECRET_NAME}" -o json | jq \
        -r '.data["ca.crt"]' | base64 -d >"${TARGET_FOLDER}/ca.crt"
    printf "done"
}

get_user_token_from_secret() {
    echo -e -n "\\nGetting user token from secret"
    USER_TOKEN=$(kubectl get secret --namespace "tenant${NAME}admin" "${SECRET_NAME}" -o json | jq -r '.data["token"]' | base64 -d)
    printf "done"
}

set_kube_config_values() {
    context=$(kubectl config current-context)
    echo -e "\\nSetting current context to: $context"

    CLUSTER_NAME=$(kubectl config get-contexts "$context" | awk '{print $3}' | tail -n 1)
    echo "Cluster name: ${CLUSTER_NAME}"

    ENDPOINT=$(kubectl config view \
        -o jsonpath="{.clusters[?(@.name == \"${CLUSTER_NAME}\")].cluster.server}")
    echo "Endpoint: ${ENDPOINT}"

    # Set up the config
    echo -e "\\nPreparing k8s-${SERVICE_ACCOUNT_NAME}-tenant${NAME}admin-conf"
    echo -n "Setting a cluster entry in kubeconfig"
    kubectl config set-cluster "${CLUSTER_NAME}" \
        --kubeconfig="${KUBECFG_FILE_NAME}" \
        --server="${ENDPOINT}" \
        --certificate-authority="${TARGET_FOLDER}/ca.crt" \
        --embed-certs=true

    echo -n "Setting token credentials entry in kubeconfig"
    kubectl config set-credentials \
        "${SERVICE_ACCOUNT_NAME}-tenant${NAME}admin-${CLUSTER_NAME}" \
        --kubeconfig="${KUBECFG_FILE_NAME}" \
        --token="${USER_TOKEN}"

    echo -n "Setting a context entry in kubeconfig"
    kubectl config set-context \
        "${SERVICE_ACCOUNT_NAME}-tenant${NAME}admin-${CLUSTER_NAME}" \
        --kubeconfig="${KUBECFG_FILE_NAME}" \
        --cluster="${CLUSTER_NAME}" \
        --user="${SERVICE_ACCOUNT_NAME}-tenant${NAME}admin-${CLUSTER_NAME}" \
        --namespace="tenant${NAME}admin"

    echo -n "Setting the current-context in the kubeconfig file.."
    kubectl config use-context "${SERVICE_ACCOUNT_NAME}-tenant${NAME}admin-${CLUSTER_NAME}" \
        --kubeconfig="${KUBECFG_FILE_NAME}"

    rm "${TARGET_FOLDER}/ca.crt"
}

# Adding the tenant-admin in tenant specs
edit_tenant_sample() {
    NAME=$1
    echo -e "\\nEditing the tenant-${NAME}-cr "
    kubectl get tenant tenant-"${NAME}"-cr -o yaml >"${TARGET_FOLDER}"/tenant-sample.yaml
    sed -ri "s/t1-user1/${SERVICE_ACCOUNT_NAME}/" "${TARGET_FOLDER}"/tenant-sample.yaml
    sed -ri "s/XXXXX/tenant${NAME}admin/" "${TARGET_FOLDER}"/tenant-sample.yaml
    sed -ri "s/default/tenant${NAME}admin/" "${TARGET_FOLDER}"/tenant-sample.yaml
    kubectl apply -f "${TARGET_FOLDER}"/tenant-sample.yaml
    rm "${TARGET_FOLDER}/tenant-sample.yaml"
}

# Creating the tenant namespace from tenant namespace CRD
create_tenantnamespace_sample() {
    NAME=$1
    echo -e "\\nCreating tenantnamespace-sample"
    FILE="/tmp/kube/TENANT_NAMESPACE_SAMPLE.yaml"
    sleep 5
    if test -f "$FILE"; then
        kubectl apply -f "$FILE"
        return
    fi
    wget -O "${FILE}" "${TENANT_NAMESPACE_SAMPLE}"
    sed -ri "s/sample/${NAME}-cr/; s/t1-ns1/t${NAME}-ns${NAME}/; s/tenant1admin/tenant${NAME}admin/" "${TARGET_FOLDER}"/TENANT_NAMESPACE_SAMPLE.yaml
    echo -e "\\nCreating tenantnamespace from ${TENANT_NAMESPACE_SAMPLE}"
    KUBECONFIG="${KUBECFG_FILE_NAME}" kubectl apply -f "${FILE}"
    rm "${TARGET_FOLDER}/TENANT_NAMESPACE_SAMPLE.yaml"
}

create_sa() {
    NAME=$1
    create_service_account "${NAME}"
    get_secret_name_from_service_account "${NAME}"
    extract_ca_crt_from_secret "${NAME}"
    get_user_token_from_secret "${NAME}"
    set_kube_config_values "${NAME}"
    edit_tenant_sample "${NAME}"
    create_tenantnamespace_sample "${NAME}"

    echo -e "\\nTest with:"
    echo "KUBECONFIG=${KUBECFG_FILE_NAME} kubectl get pods"
    echo "you should not have any permissions by default - you have just created the authentication part"
    echo "You will need to create RBAC permissions"
}

while_loop() {
    FUNCTION=$1
    COUNT=$2
    START=0
    while [ "$START" != "$COUNT" ]; do
        "${FUNCTION}" "${START}"
        START=$((START + 1))
    done

}

create_target_folder
create_tenant_crd
create_tenantnamespace_crd
create_tenant_controller_manager
while_loop create_tenant_sample "${COUNT}"
while_loop create_sa "${COUNT}"
