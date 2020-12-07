# HNC Performance

## Before you start

This performance test is to test HNC's startup performance, including controller
working time and number of object reads and writes during HNC startup and restart.

Please be aware that the default performance test will create about maximum 500
namespaces and 1000 objects in your cluster. To change the scale, you can edit
`load-topology-` files to update the namespace numbers by `N` and object numbers
by `O`.

There are three topologies we support in this test to simulate possible use cases:
```
Wide - 1 root and N children (2-level-hierarchy)
Full - 1 root, N children and N*N grandchildren (3-level-hierarchy)
Skewer - 1 root, 1 child, 1 grandchild, 1 great grandchild, ... for N generations (N-level-hierarchy)
```

## Run performance test

First, you will need to set the `enable-test-log` flag so that HNC can generate
logs for this test to catch.

To do so, add `- --enable-test-log` in `config/manager/manager.yaml` so
container `args` will look like this:
```
args:
  ...
- "--max-reconciles=10"
- "--apiserver-qps-throttle=50"
- "--enable-test-log"        ####### <<<<---- Add this line
```

Run `make manifests` to update the manifest to include the flag.

Now you can run performance test under the HNC root directory by:
```
$ scripts/performance/test.sh
```

You are expected to see a lot of namespace/role/rolebinding creation and
eventually get a report like this:
```
HNC startup time : 2020-12-04T15:42:09
Current time : 2020-12-04T15:42:22
Controllers start working time : 2020-12-04T15:42:14
Controllers finish working time : 2020-12-04T15:42:19
Controllers working time for HNC startup: 5s
Total HierConfig reconciles: 33
Total Object reconciles: 217
Total HierConfig writes: 21
Total Namespace writes: 21
Total Object writes: 50
```

#### How to measure memory

To measure realtime memory usage, you can load one topology, deploy HNC and
monitor the memory usage change manually. On GKE, you can find the realtime
CPU and memory usages if you click the name of the workload in `Workloads` tab.
It will show you the used memory compared with the limits and requests.

Here's an example:
```
$ scripts/performance/load-topology-full.sh
$ kubectl apply -f manifests/hnc-manager.yaml

### View memory usages now ###

# Cleanup namespaces
$ scripts/performance/clean-up-topologies.sh
```
