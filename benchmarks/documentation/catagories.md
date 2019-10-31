# Benchmark Categories

**Control Plane Protection**

Checks for cluster configuration settings and runtime protection of cluster resources. These checks require access to the API Server process settings via mounted host directories, assuming the cluster components are installed directly on the host. 


**Tenant Protection**

Checks for required namespace configuration settings and isolation across tenants. These checks require cluster-admin access to the namespaces under test.


**Network Segmentation and Isolation**

Checks for network security to provide isolation across tenant namespaces for ingress and egress traffic.


**Host Protection**

Checks to ensure that container hosts are protected from tenant workloads.


**Data Protection**

Checks to ensure that tenant data, including volumes and secrets, cannot be accessed by other tenants. 


**Fairness**

Checks to ensure fair usage of shared resources.


**Self-Service Operations**

Checks to verify if a cluster administrator can manage namespace level resources.
