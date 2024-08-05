# resource-replicator-operator

Kubernetes Operator that replicates kubernetes configmaps and secrets to selected namespaces

## Description

The Custom Resource Definitions (CRDs) exposes 2 APIs:

1. `ConfigMapSync` that syncs your configmaps from `sourceNamespace` to `destinationNamespace`
1. `SecretSync` that syncs your secrets from `sourceNamespace` to `destinationNamespace`

## Installation

```sh
kubectl apply -f https://raw.githubusercontent.com/Jason-CKY/resource-replicator-operator/v1.0.0/config/releases/resource-replicator-operator-v1.0.0.yaml
```

## Usage

### Replicates with annotations

#### Name-based

This allows you to either specify your `destinationNamespace` by name or by regular expression (which should match the namespace name). The value of this annotation should contain a comma separated list of permitted namespaces or regular expressions. (Example: `namespace-1,my-ns-2,app-ns-[0-9]*` will replicate only into the namespaces `namespace-1` and `my-ns-2` as well as any namespace that matches the regular expression `app-ns-[0-9]*`).

Example:

```yaml
apiVersion: apps.replicator/v1
kind: SecretSync
metadata:
  labels:
    app.kubernetes.io/name: resource-replicator-operator
    app.kubernetes.io/managed-by: kustomize
  name: secretsync-sample
spec:
  sourceNamespace: default
  destinationNamespace: my-ns-1,namespace-[0-9]*
  secretName: test-secret
```

or

```yaml
apiVersion: apps.replicator/v1
kind: ConfigMapSync
metadata:
  labels:
    app.kubernetes.io/name: resource-replicator-operator
    app.kubernetes.io/managed-by: kustomize
  name: configmapsync-sample
spec:
  sourceNamespace: default
  destinationNamespace: my-ns-1,namespace-[0-9]*
  configMapName: test-cm
```

### Cleaning up abandoned resource

Once the source resource has been deleted, all the replicated resources will also be cleaned up by this process. 

Updating the source resource's replication annotation will also update the replicated resource.
Example:

Updating

```yaml
apiVersion: apps.replicator/v1
kind: ConfigMapSync
metadata:
  labels:
    app.kubernetes.io/name: resource-replicator-operator
    app.kubernetes.io/managed-by: kustomize
  name: configmapsync-sample
spec:
  sourceNamespace: default
  destinationNamespace: my-ns-1,namespace-[0-9]*
  configMapName: test-cm
```

to

```yaml
apiVersion: apps.replicator/v1
kind: ConfigMapSync
metadata:
  labels:
    app.kubernetes.io/name: resource-replicator-operator
    app.kubernetes.io/managed-by: kustomize
  name: configmapsync-sample
spec:
  sourceNamespace: default
  destinationNamespace: namespace-[0-9]*
  configMapName: test-cm
```

will cause the configmap in `my-ns-1` to be removed.

## Getting Started

### Prerequisites

- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/resource-replicator-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/resource-replicator-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/resource-replicator-operator:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/resource-replicator-operator/<tag or branch>/dist/install.yaml
```

## Contributing

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

```txt
    http://www.apache.org/licenses/LICENSE-2.0
```

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
