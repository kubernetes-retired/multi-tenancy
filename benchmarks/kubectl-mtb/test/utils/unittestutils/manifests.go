package unittestutils

const DisallowPrivilegedContainers = `
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: disallow-privileged
spec:
  validationFailureAction: enforce
  rules:
    - name: validate-privileged
      match:
        resources:
          kinds:
            - Pod
          namespaces:
            - t1-ns1
      validate:
        message: "Privileged mode is not allowed. Set privileged to false"
        pattern:
          spec:
            containers:
              - =(securityContext):
                  # https://github.com/kubernetes/api/blob/7dc09db16fb8ff2eee16c65dc066c85ab3abb7ce/core/v1/types.go#L5707-L5711
                  # k8s default to false
                  =(privileged): false
`
const DisallowAddCapabilities = `
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: disallow-new-capabilities
spec:
  validationFailureAction: enforce
  rules:
  - name: validate-add-capabilities
    match:
      resources:
        kinds:
        - Pod
        namespaces:
        - t1-ns1
    validate:
      message: "New capabilities cannot be added"
      anyPattern:
      - spec:
          containers:
          - name: "*"
            =(securityContext):
              =(capabilities):
                X(add): null
`

const DisallowBindMounts = `
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata: 
  name: disallow-bind-mounts
spec: 
  validationFailureAction: enforce
  rules: 
  - name: validate-hostPath
    match: 
      resources: 
        kinds: 
        - Pod
        namespaces:
        - t1-ns1
    validate: 
      message: "Host path volumes are not allowed"
      pattern: 
        spec: 
          =(volumes): 
          - X(hostPath): null
`

const DisallowHostIPC = `
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: disallow-host-ipc
spec:
  validationFailureAction: enforce
  rules:
  - name: validate-hostIPC
    match:
      resources:
        kinds:
        - Pod
        namespaces:
        - t1-ns1
    validate:
      message: "Use of host IPC namespaces is not allowed"
      pattern:
        spec:
          =(hostIPC): "false"
`

const DisallowHostPID = `
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: disallow-host-pid
spec:
  validationFailureAction: enforce
  rules:
  - name: validate-hostPID
    match:
      resources:
        kinds:
        - Pod
        namespaces:
        - t1-ns1
    validate:
      message: "Use of host PID namespaces is not allowed"
      pattern:
        spec:
          =(hostPID): "false"
`

const DisallowNetworkPorts = `
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: disallow-host-network-port
spec:
  validationFailureAction: enforce
  rules:
  - name: validate-host-network
    match:
      resources:
        kinds:
        - Pod
        namespaces:
        - t1-ns1
    validate:
      message: "Use of hostNetwork is not allowed"
      pattern:
        spec:
          =(hostNetwork): false
  - name: validate-host-port
    match:
      resources:
        kinds:
        - Pod
        namespaces:
        - t1-ns1
    validate:
      message: "Use of hostPort is not allowed"
      pattern:
        spec:
          containers:
          - name: "*"
            =(ports):
              - X(hostPort): null
`

const DisallowNodePortServices = `
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: restrict-nodeport
spec:
  validationFailureAction: enforce
  rules:
  - name: validate-nodeport
    match:
      resources:
        kinds:
        - Service
        namespaces:
        - t1-ns1
    validate:
      message: "Services of type NodePort are not allowed"
      pattern: 
        spec:
          type: "!NodePort"
`

const DisallowPrivilegedEscalation = `
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: disallow-allow-privilege-escalation
spec:
  validationFailureAction: enforce
  rules:
  - name: validate-allowPrivilegeEscalation
    match:
      resources:
        kinds:
        - Pod
        namespaces:
        - t1-ns1
    validate:
      message: "Privileged mode is not allowed. Set allowPrivilegeEscalation to false"
      pattern:
        spec:
          containers:
          - =(securityContext):
              # https://github.com/kubernetes/api/blob/7dc09db16fb8ff2eee16c65dc066c85ab3abb7ce/core/v1/types.go#L5754
              =(allowPrivilegeEscalation): false
`

const DisallowRootUser = `
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: disallow-root-user
spec:
  validationFailureAction: enforce
  rules:
  - name: validate-runAsNonRoot
    match:
      resources:
        kinds:
        - Pod
        namespaces:
        - t1-ns1
    validate:
      message: "Running as root user is not allowed. Set runAsNonRoot to true"
      anyPattern:
      - spec:
          securityContext:
            runAsNonRoot: true
      - spec:
          containers:
          - securityContext:
              runAsNonRoot: true
`

