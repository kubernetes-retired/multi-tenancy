# Benchmark Types

There are two types of benchmark checks envisioned:

**Configuration Checks**

Configuration checks will audit the configuration of Kubernetes components and resources. For example, a configuration check can ensure that an API server flag is enabled.


**Behavioral Checks**

Behavioral checks will verify runtime compliance for a desired outcome. These checks will involve running automated tests in configured namespaces that are being tested for multi-tenancy.