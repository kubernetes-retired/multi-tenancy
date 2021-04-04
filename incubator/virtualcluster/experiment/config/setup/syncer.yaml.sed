apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: vc-syncer-role
rules:
- apiGroups:
    - ""
  resources:
    - configmaps
    - endpoints
    - namespaces
    - pods
    - secrets
    - services
    - serviceaccounts
    - persistentvolumeclaims
  verbs:
    - get
    - list
    - watch
    - create
    - update
    - patch
    - delete
- apiGroups:
    - extensions
  resources:
    - ingresses
  verbs:
    - get
    - list
    - watch
    - create
    - update
    - patch
    - delete
    - deletecollection
- apiGroups:
    - scheduling.k8s.io
  resources:
    - priorityclasses
  verbs:
    - get
    - list
    - watch
    - create
    - update
    - patch
    - delete
- apiGroups:
    - ""
    - storage.k8s.io
  resources:
    - events
    - nodes
    - persistentvolumes
    - storageclasses
  verbs:
    - get
    - list
    - watch
- apiGroups:
    - ""
    - storage.k8s.io
  resources:
    - events
  verbs:
    - create
    - patch
- apiGroups:
    - ""
  resources:
    - namespaces/status
    - pods/status
    - services/status
    - nodes/status
    - persistentvolumes/status
    - persistentvolumeclaims/status
  verbs:
    - get
- apiGroups:
    - tenancy.x-k8s.io
  resources:
    - virtualclusters
  verbs:
    - get
    - list
    - watch
- apiGroups:
    - tenancy.x-k8s.io
  resources:
    - virtualclusters/status
  verbs:
    - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vc-syncer-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: vc-syncer-role
subjects:
  - kind: ServiceAccount
    name: vc-syncer
    namespace: vc-manager
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: vc-syncer
  namespace: vc-manager
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: SYNCER_NAME
  namespace: vc-manager
  labels:
    app: SYNCER_NAME
spec:
  replicas: 1
  selector:
    matchLabels:
      app: SYNCER_NAME
  template:
    metadata:
      creationTimestamp: null
      labels:
        app: SYNCER_NAME
    spec:
      serviceAccountName: vc-syncer
      containers:
        - command:
            - syncer
            - --deployment-on-meta=true
            - --super-master-kubeconfig=/etc/supercluster/config
            - --syncer-name=SYNCER_NAME
            - --feature-gates
            - SuperClusterPooling=true
          image: virtualcluster/syncer-amd64
          imagePullPolicy: Always
          name: vc-syncer
          livenessProbe:
            failureThreshold: 3
            initialDelaySeconds: 30
            periodSeconds: 20
            successThreshold: 1
            tcpSocket:
              port: 8080
            timeoutSeconds: 1
          volumeMounts:
          - mountPath: /etc/supercluster/
            name: supercluster-config
      volumes:
      - secret:
          defaultMode: 420
          secretName: SUPER_CLUSTER_CONFIG
        name: supercluster-config
