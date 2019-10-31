# Ensure that users of Tenant A cannot connect to pods running in Tenant B

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Network Protection & Isolation

**Description:**

Tenant A must be isolated from Tenant B at the network level. If tenant B is running pods exposing TCP or UDP ports then Tenant A should not be able to reach these ports.


**Rationale:**

If Tenant A is capable of reaching pods running in Tenant B, then it could impact tenant B before running for instance a D.O.S attack.

**Audit:**

The benchmarks configuration can define the port range to test. If the port range is very large then the test can be broken down into multiple port ranges.

Start a TCP server pod in Tenant B binding to all the ports in the configured port range
Start a port scan pod in Tenant A scanning all the ports within the port range
	
The port scan must demonstrate that none of the ports exposed by the pod running in tenant B are accessible.
