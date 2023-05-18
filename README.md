# Kubernetes Operator for NATS JWT

This operator aims to ease up the decentralized configuration pattern as described in [NATS Docs](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt).
To do so, it creates three new CRDs, one for Operator, Account and User objects.

It allows to (nearly) fully customize the issued JWTs, generates a server config and provides a NATS auth server to be run next to a NATS server (cluster).

## Getting started

In order to get started, we install the operator using kustomize, afterwards create a `NatsOperator` object.

```yaml
apiVersion: nats.deinstapel.de/v1alpha1
kind: NatsOperator
metadata:
  namespace: nats-cluster
  name: root-operator
spec:
  signingKeys: [] # Optionally can specify external operator scoped signing keys here.
```

The operator will start to reconcile the NatsOperator by:
1. Creating a KeyPair for the Operator used as Root of Trust.
2. Create a NatsAccount and a NatsUser object for the shipped AccountServer
3. Create a config file that can be mounted into the NATS server cluster where the Root of Trust and the System account is configured.

In order to enable the configuration, include `auth.conf` in your server config file.

## Usage

### Creating an account

Afterwards, you can create NATS Accounts and users within the accounts.

An Account can be created as follows:
```yaml
apiVersion: nats.deinstapel.de/v1alpha1
kind: NatsAccount
metadata:
  namespace: nats-cluster # Must be the same namespace as the operator
  name: app-account
spec:
  operatorRef:
    name: root-operator # Setup the signing key for this account
  allowedUserNamespaces:
  - app-namespace # Defines the kubernetes namespaces where NatsUser objects for this account will be valid
  imports: []
  exports: []
  limits: 
    # The default limits are 0 for all items, so a user will not be allowed to connect or subscribe
    # Temporary allow unlimited connections, subscriptions and payload sizes. 
    # TODO User: Adapt this for your app
    conn: -1
    imports: -1
    exports: -1
    subs: -1
    payload: -1
    data: -1
```

### Creating a user

Once you've created an account, it's time to generate a User object.
Now here you can also directly inline the permissions.

```yaml
apiVersion: nats.deinstapel.de/v1alpha1
kind: NatsUser
metadata:
  namespace: app-namespace
  name: app-backend
spec:
  accountRef:
    namespace: nats-cluster
    name: app-account
  limits:
    payload: -1
    subs: -1
    data: -1
  permissions:
    sub:
      allow:
      - "app.input.>"
      - "app.process.data"
    pub:
      allow:
      - "app.output.>"
    resp:
      # Allow request/reply
      max: 1
      ttl: -1
```

The operator will create:
1. A NKey pair
2. A JWT signed by the referenced Account
3. A user.creds file that can directly be passed into libraries

The user.creds file and the Seed keys will be stored in a secret named like the NatsUser object.

If a user is edited at runtime, the operator will reissue the JWT.

In the future, the operator also will revoke all old JWTs issued for this user.

### Integrating with NATS Helm Chart

If you want to use the above manifests with a theoretical NATS helm setup, you can use something like the following values.yaml settings to include the generated manifests:

```yaml
config:
  merge:
    # Include the auth.conf our operator wrote to a secret into the server.
    00$include: "../custom-auth/auth.conf"

# Patch the containers and statefulSet to include the generated authentication config.
container:
  patch:
  - op: add
    path: "/volumeMounts/-"
    value:
      name: auth-config
      mountPath: "/etc/custom-auth"
statefulSet:
  patch:
  - op: add
    path: /spec/template/spec/volumes/-
    value:
      name: "auth-config"
      secret:
        defaultMode: 420
        # The secret name is ${operatorCrdName}-server-config
        secretName: "root-operator-server-config"
```

After you've done this, the Operator from your CRD is anchored into the NATS server, with the system-account preloaded in the configuration.
Now you can proceed to deploy your account-server, you can use manifests like this:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: nats-account-server
  namespace: nats-cluster
rules:
- apiGroups: ["nats.deinstapel.de"]
  resources: ["natsaccounts"]
  verbs: ["get", "list", "watch"]

---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: nats-account-server
  namespace: nats-cluster

---

apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: nats-account-server
  namespace: nats-cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: nats-account-server
subjects:
- kind: ServiceAccount
  name: nats-account-server
  namespace: nats-cluster

---

apiVersion: apps/v1
kind: Deployment
metadata:
  name: nats-account-server
  namespace: nats-cluster
spec:
  strategy:
    type: RollingUpdate
  selector:
    matchLabels:
      app.kubernetes.io/component: nats-account-server
  replicas: 1
  template:
    metadata:
      labels:
        app.kubernetes.io/component: nats-account-server
    spec:
      serviceAccountName: nats-account-server
      containers:
      - name: account-server
        image: "ghcr.io/deinstapel/nats-jwt-operator/account-server:edge"
        args: ["--metrics-bind-address", ":12003", "--health-probe-bind-address", ":12002"]
        env:
        - name: "NATS_URL"
          value: "nats://${NATS_RELEASE_NAME}-nats-headless.nats-cluster.svc.cluster.local"
        - name: "NATS_CREDS_FILE"
          value: "/etc/nats/user.creds"
        - name: "POD_NAMESPACE"
          valueFrom:
            fieldRef:
              fieldPath: "metadata.namespace"
        volumeMounts:
        - name: "credentials"
          mountPath: "/etc/nats"
          readOnly: true
        securityContext:
          capabilities:
            add:
            - NET_BIND_SERVICE
            drop:
            - all
          runAsUser: 0
          runAsGroup: 0
      volumes:
      - name: "credentials"
        secret:
          defaultMode: 420
          secretName: "root-operator-jwt"
          items:
          - key: "user.creds"
            path: "user.creds"
            mode: 420
```

This will run a service that's connecting to NATS, watches all K8s NatsAccount resources for the given operator
and actively pushes them towards the NATS server, as well as subscribes to the Lookup topic as described [here](https://docs.nats.io/running-a-nats-service/configuration/securing_nats/auth_intro/jwt/resolver#nats-based-resolver-integration).

### Integrating with Nats Controllers for Kubernetes (NACK)

If you also want to declaratively manage NATS JetStream resources, the manifests below show a basic example of how to use the generated NATS User JWT in combination with the NACK Account resource to authorize to the NATS server to manage streams.

The first step is to create a jetstream-admin user with this operator in the account in-question:

```yaml

apiVersion: nats.deinstapel.de/v1alpha1
kind: NatsUser
metadata:
  name: app-jetstream-admin
  namespace: app-namespace
spec:
  # Reference the NatsAccount here to issue the correct JWT
  accountRef:
    namespace: nats-cluster
    name: app-account
  limits:
    payload: -1
    subs: -1
    data: -1
  permissions:
    sub:
      # FIXME: NACK doesn't allow us to use a custom _INBOX prefix
      allow:
      - "_INBOX.>"
      - "$JS.>"
    pub:
      allow:
      - "$JS.>"
    resp:
      max: -1
      ttl: -1
```

After you've created the user, create an Account resource from NACK, telling NACK the Server URLs and the JWT to authorize with:

```yaml
---

# This one is using the previously generated account to create streams and consumers
apiVersion: jetstream.nats.io/v1beta2
kind: Account
metadata:
  name: app-jetstream-admin
  namespace: app-namespace
spec:
  name: app-jetstream-admin
  servers:
  - nats://${NATS_RELEASE_NAME}-nats-headless.nats-cluster.svc.cluster.local:4222
  creds:
    # Pull in the generated user.creds file from the NatsUser we've created above.
    secret:
      name: app-jetstream-admin
    file: "user.creds"
```

Then you're ready to manage streams and consumers by referencing the Account resource there again:

```yaml
apiVersion: jetstream.nats.io/v1beta2
kind: Stream
metadata:
  name: app-stream
  namespace: app-namespace
spec:
  name: app-stream
  subjects:
  - "app.foobar.>"
  storage: file
  maxAge: 1h
  description: "Stores the stream of foobars for app xyz"
  replicas: 1
  account: app-jetstream-admin
```

This way, you can declaratively and securely manage not only your users but also the Streams and Consumers

#### Known issues

Currently, in NATS NACK the consumer resource does not respect the user.creds file passed in via the account.


## Running on the cluster

### Helm 

A helm chart is provided in deploy/charts

### Manually / dev

1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```


2. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=<some-registry>/nats-jwt-operator:tag
```

3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/nats-jwt-operator:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller from the cluster:

```sh
make undeploy
```

## Contributing

PR's welcome!

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

