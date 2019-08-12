# Kubernetes Multi-tenancy profile: Single Tenant Cluster

In order to define a secure multi-tenant cluster, we must first agree upon a baseline for a secure single tenant cluster. 

## Overall functional model summary

1. A **Tenant** is defined as a team of users/ identities that shall have exclusive use of one or more resources of a Kubernetes cluster in parallel with users of other tenants using their own separate and isolated resources within the same cluster. We will interchangeably use the terms \"users\" or \"members\" of a tenant.

2. In this single tenant profile, a Kubernetes cluster and all of its objects and resources are explicitly owned only by a single tenant. There is no sharing of the cluster or attempt to wall off objects and resources between different tenant users.

## Applicable benchmarks used to define a secure Kubernetes cluster

1. NIST SP 800-190: Application Container Security Guide https://csrc.nist.gov/publications/detail/sp/800-190/final 
1. CIS Benchmark for Kubernetes 1.4 https://csrc.nist.gov/publications/detail/sp/800-190/final

## Kubernetes cluster configuration requirements for profile single tenant

1. The recommended minimum version of Kubernetes release is 1.15. Earlier kubernetes releases may be used with some potential restrictions.  (Note: Maybe better to list api versions required for various Kubernetes resources/ apis).
1. Kubernetes Role Based Access Control RBAC must be supported and enabled.
    1. Set up cluster roles 
    1. Give each user the least permissive role possible to achieve their desired outcome 
    1. Do not give `write` access to `kubesystem` or other centrally managed namespaces
    1. Prefer `Roles` and `RoleBindings` to `ClusterRoles` and `ClusterRoleBindings` because they are scoped to a namespace
1. Alternately a functional equivalent of Kubernetes RBAC may be supported via alternative authorization mechanisms such as Open Policy Agent based access control as long as the behavior and requirements listed below are met.
1. Attribute Based Access Control or static file based access control options should be disabled on the cluster.
1. The following set of Kubernetes built-in admission controllers should be enabled.  Functionally equivalent admission control via alternative mechanisms such as Open Policy Agent or other custom admission controllers may be used as long as they are functionally equivalent to the requirements listed here.
    * PodSecurityPolicy
      Enforce _MustRunAsNonRoot pod security policy_: this forces containers to run as non-root
      ```
      runAsUser:
      rule: 'MustRunAsNonRoot'
      ```
    * Require mandatory access control by configuring to use AppArmor or SELinux
    
      * AppArmor: 
      
        ```
        annotations:
        seccomp.security.alpha.kubernetes.io/allowedProfileNames: 'docker/default'
        apparmor.security.beta.kubernetes.io/allowedProfileNames: 'runtime/default'
        seccomp.security.alpha.kubernetes.io/defaultProfileName:  'docker/default'
        apparmor.security.beta.kubernetes.io/defaultProfileName:  'runtime/default'
        ```
      
      * SELinux: 
    
         ```
         securityContext:
         seLinuxOptions:
         level: "s0:c123,c456"
         ```
    * Block the majority of users from using HostPath volume mounting. Volumes should be exclusively mounted using Cinder.
    * AlwaysPullImages: modifies all pods to set their `PullPolicy` to `Always`. This checks the upstream server to make sure the image is correct, and invokes an access check. This makes sure revoked images are not being run. 
    * NodeRestriction
    * ServiceAccount
    * ResourceQuota
    * LimitRanger
    * Portieris: enforce policies on container images to ensure only trusted images are used. This admission controller uses the Notary project to ensure images are trusted and restricts repositories they come from. https://github.com/IBM/portieris Users should not enable insecure registries.
1. Container Networking  
    * The container networking plugin used in the cluster must support Kubernetes Network policy at version v1beta1 at a minimum.
    * Calico provides appropriate traffic segregation and isolation, as do other CNIs
    * Use `NetworkPolicy` to restrict pod to pod traffic. Set a `default-deny` policy as a starting point. 
      ```
      apiVersion: networking.k8s.io/v1
      kind: NetworkPolicy
      metadata:
        name: default-deny
      spec:
        podSelector: {}
      policyTypes:
        - Ingress
      ```
    * Restrict Ingress to as few nodes as possible, and those nodes should be the only ones registered with the external network load balancer.
    * Use firewall rules to block inter-node connections so that compromising one node does not compromise an entire cluster.
1. Authentication: 
    * Use OIDC for cluster authentication 
    * Set up `encryption-provider-config` for `encryption-at-rest` of Kubernetes secrets 
    * Use an external key management service to provision symmetric encryption keys, e.g. Hashicorp Vault or AWS KMS
1. Secure runtime / node isolation 
    * Use a hardened container runtime 
    * Enable seccomp
    * Enable apparmor 
1. Establishing orchestrator node trust 
    * Use `kubeadm` with TLS bootstrapping to ensure the identity of nodes in the cluster 
    * Use `kubeadm token create --ttl 5m` to create short-lived, per-node tokens
    * These tools are compatible with a Terraform + config management (Ansible) tool, or a ClusterAPI provider
1. Host OS Access
    * Strictly restrict number of users who can SSH to host machines 
    * Require multi-factor authentication 
 



