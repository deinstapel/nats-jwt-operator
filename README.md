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


### Running on the cluster
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
// TODO(user): Add detailed information on how you would like others to contribute to this project

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

