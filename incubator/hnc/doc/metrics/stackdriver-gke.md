# Stackdriver on GKE

To view HNC Metrics in Stackdriver, you will need a GKE cluster with HNC installed
and a method to access Cloud APIs, specifically Stackdriver monitoring APIs, from GKE.
We will introduce two methods and their pros and cons:
* (Recommended) Use the [Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)
, which has improved security properties and manageability. Please note that it’s
in a pre-release state (Beta) and might change.
* Use the [Compute Engine default service account](https://cloud.google.com/compute/docs/access/service-accounts#default_service_account)
on your GCE nodes, which is easy to set up but can result in over-provisioning of permissions.

Once it's set up, you can view the metrics in Stackdriver  [Metrics Explorer](https://cloud.google.com/monitoring/charts/metrics-explorer)
by searching the metrics keywords.

## Option 1: Use Workload Identity (Recommended)
These are the steps to set up HNC in GKE using Workload Identity, which are further described below:
1. Ensure your GKE clusters have Workload Identity enabled
2. Install HNC
3. Create a suitable GSA and map it to the KSA

### Ensure your GKE clusters have Workload Identity enabled
This section introduces how to enable [Workload Identity (WI)](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) either on a new cluster or on an existing cluster:
* [Enable WI on a new cluster](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity#enable_workload_identity_on_a_new_cluster)
* [Enable WI on an existing cluster](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity#enable_workload_identity_on_an_existing_cluster)

### Install HNC
We will install HNC and make sure the `hnc-system/default` Kubernetes service account
exists. Here are the steps:
1. [Install HNC](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/incubator/hnc#installing-or-upgrading-hnc)
2. Run `kubectl get serviceaccounts -n hnc-system` and make sure the `default` is listed:
```
     NAME      SECRETS   AGE
     default   1         5d
```

### Create a suitable GSA and map it to the KSA
This section will create an [Cloud IAM policy binding](https://cloud.google.com/sdk/gcloud/reference/iam/service-accounts/add-iam-policy-binding)
between the Kubernetes service account (KSA) and the GCP service account (GSA).
This binding allows the KSA to act as the GSA so that the HNC metrics can be exported
to Stackdriver. The above `hnc-system/default` is the KSA to be used. This action
requires Security Admin role, with `iam.serviceAccounts.setIamPolicy` permission,
which your User Account should already have if you have the full-access Owner role.
Therefore, you can execute the following commands to add the IAM policy binding.
     
Steps:
1. [Create a Google service account (GSA)](https://cloud.google.com/docs/authentication/production#creating_a_service_account):
    ```bash
    gcloud iam service-accounts create [GSA_NAME]
    ```
2. Grant “[Monitoring Metric Writer](https://cloud.google.com/monitoring/access-control#mon_roles_desc)”
role to the GSA:
    ```bash
    gcloud projects add-iam-policy-binding [PROJECT_ID] --member \
      "serviceAccount:[GSA_NAME]@[PROJECT_ID].iam.gserviceaccount.com" \
      --role "roles/monitoring.metricWriter"
    ```
3. Create an [Cloud IAM policy binding](https://cloud.google.com/sdk/gcloud/reference/iam/service-accounts/add-iam-policy-binding)
between `hnc-system/default` KSA and the newly created GSA:
     ```
     gcloud iam service-accounts add-iam-policy-binding \
       --role roles/iam.workloadIdentityUser \
       --member "serviceAccount:[PROJECT_ID].svc.id.goog[hnc-system/default]" \
       [GSA_NAME]@[PROJECT_ID].iam.gserviceaccount.com
   ```
4. Add the `iam.gke.io/gcp-service-account=[GSA_NAME]@[PROJECT_ID]` annotation to
the KSA, using the email address of the Google service account:
     ```
     kubectl annotate serviceaccount \
       --namespace hnc-system \
       default \
       iam.gke.io/gcp-service-account=[GSA_NAME]@[PROJECT_ID].iam.gserviceaccount.com
   ```
5. Verify the service accounts are configured correctly by creating a Pod with the
Kubernetes service account that runs the `cloud-sdk` container image, and connecting
to it with an interactive session:
     ```
     kubectl run --rm -it \
       --generator=run-pod/v1 \
       --image google/cloud-sdk:slim \
       --serviceaccount default \
       --namespace hnc-system \
       workload-identity-test
   ```
6. You are now connected to an interactive shell within the created Pod. Run the following command:
     ```
     gcloud auth list
   ```
   If the service accounts are correctly configured, the GSA email address is listed
   as the active (and only) identity. 

## Option 2: Use GCE default Service Account
By default, GKE clusters without Workload Identity use the GCE default Service Account,
and since this SA already has permission to write metrics to Stackdriver, no extra
steps are required other than creating a GKE cluster and installing HNC. The HNC
workloads running on the GCE nodes will use their Service Accounts by default.