# NATS JWT Operator

## Usage

[Helm](https://helm.sh) must be installed to use the charts.  Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

  helm repo add nats-jwt https://deinstapel.github.io/nats-jwt-operator

If you had already added this repo earlier, run `helm repo update` to retrieve
the latest versions of the packages.  You can then run `helm search repo
nats-jwt` to see the charts.

To install the nats-jwt-operator chart:

    helm install nats-jwt-operator nats-jwt/nats-jwt-operator

To uninstall the chart:

    helm delete nats-jwt-operator
