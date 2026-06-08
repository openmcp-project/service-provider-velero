[![REUSE status](https://api.reuse.software/badge/github.com/openmcp-project/service-provider-velero)](https://api.reuse.software/info/github.com/openmcp-project/service-provider-velero)

# 🛡️ service-provider-velero

A service provider for managing [Velero](https://velero.io/) backup and restore capabilities within a ManagedControlPlane environment. This provider enables disaster recovery and data protection by automatically installing and configuring Velero on workload clusters.

## Quality Criteria

[![Quality: Incubating](https://img.shields.io/badge/Quality-Incubating-3d9970?style=flat-square&labelColor=555)](https://open-control-plane.io/developers/serviceprovider/quality-criteria)

| Criterion                         | Status  | Notes                                                                                                                                                                                                                              |
| --------------------------------- | :----:  | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Deletion behaviour                |   ⚠️    | A finalizer ensures the Service Provider managed resources like `Deployments` etc. But there is no behaviour that ensures deletion is blocked if custom resources (e.g. Velero' `Backup` objects) in a `ControlPlane` still exist. |
| Status reporting & error messages |   ✅    |                                                                                                                                                                                                                                    |
| Operation annotations             |   ❌    | `openmcp.cloud/operation` (pause / force-reconcile) annotations are not honoured.                                                                                                                                                  |
| API stability policy              |   ✅    |                                                                                                                                                                                                                                    |
| Custom CA support                 |   ❌    | Custom CA bundle propagation to Velero components is not implemented.                                                                                                                                                              |
| Release artifacts (image + OCM)   |   ✅    |                                                                                                                                                                                                                                    |
| Testing                           |   ✅    |                                                                                                                                                                                                                                    |
| Ownership and maintenance docs    |   ✅    |                                                                                                                                                                                                                                    |

See the [OpenControlPlane Quality Criteria](https://open-control-plane.io/developers/serviceprovider/quality-criteria) for definitions.

## 📖 Overview

The Velero service provider automates the lifecycle management of Velero installations, including:

- 💾 **Backup & Restore** - Deploys Velero server for Kubernetes backup and disaster recovery
- 🔌 **Plugin Support** - Install cloud provider plugins (AWS, Azure, GCP, etc.)
- 🔐 **Air-Gapped Support** - Full support for private registries and air-gapped environments
- 🔄 **Drift Detection** - Automatic reconciliation with configurable poll intervals
- 📊 **Status Tracking** - Real-time status reporting of all managed resources

## 🏗️ Architecture

```
Platform Cluster                  Workload Cluster              ManagedControlPlane
┌──────────────────────────┐     ┌────────────────────┐       ┌────────────────────┐
│  openmcp-system          │     │  Tenant Namespace  │       │  velero            │
│  ┌────────────────────┐  │     │  ┌──────────────┐  │       │  ┌──────────────┐  │
│  │ service-provider-  │  │     │  │ Velero       │  │       │  │ Velero CRDs  │  │
│  │ velero             │──┼────▶│  │ Server       │──┼──────▶│  │ (Backup,     │  │
│  │                    │──┼─────┼──┼──────────────┼──┼──────▶│  │  Restore...) │  │
│  └────────────────────┘  │     │  │ + Plugins    │  │       │  └──────────────┘  │
│  ┌────────────────────┐  │     │  └──────────────┘  │       └────────────────────┘
│  │ ProviderConfig     │  │     └────────────────────┘
│  │ imagePullSecrets   │  │
│  └────────────────────┘  │
└──────────────────────────┘
```

The Velero server runs on the **workload cluster**, while the Velero CRDs (Backup, Restore, Schedule, etc.) are installed on the **ManagedControlPlane** for tenant isolation.

> 📡 **Note:** The Kubernetes nodes on the workload cluster where Velero runs must be able to resolve and reach any configured [backup storage locations](https://velero.io/docs/main/locations/).

## 🚦 Getting Started

### Prerequisites

- Go 1.21+
- [Task](https://taskfile.dev/) (task runner)
- Docker (for building images)
- Access to an openMCP environment

### 🧪 Running End-to-End Tests

```bash
task test-e2e
```

This uses the [openmcp-testing](https://github.com/openmcp-project/openmcp-testing) framework to spin up a full test environment.

## 📝 API Reference

### Velero

The `Velero` resource represents a Velero installation for a ManagedControlPlane.

```yaml
apiVersion: velero.services.openmcp.cloud/v1alpha1
kind: Velero
metadata:
  name: my-velero
  namespace: default
spec:
  version: "v1.17.2"
  plugins:
    - name: "aws"
      version: "v1.13.2"
```

| Field                    | Type     | Description                               |
| ------------------------ | -------- | ----------------------------------------- |
| `spec.version`           | string   | The version of Velero to install          |
| `spec.plugins`           | []Plugin | List of plugins to install with Velero    |
| `spec.plugins[].name`    | string   | Plugin name (e.g., `aws`, `azure`, `gcp`) |
| `spec.plugins[].version` | string   | Plugin version                            |

### ProviderConfig

The `ProviderConfig` resource configures global settings for all Velero deployments.

```yaml
apiVersion: velero.services.openmcp.cloud/v1alpha1
kind: ProviderConfig
metadata:
  name: velero
spec:
  pollInterval: 15m
  availableImages:
    - name: velero
      versions: ["v1.17.2", "v1.16.2"]
      image: "velero/velero"
    - name: aws
      versions: ["v1.13.2", "v1.12.2"]
      image: "velero/velero-plugin-for-aws"
  imagePullSecrets:
    - name: privateregcred
```

| Field                             | Type        | Description                                               |
| --------------------------------- | ----------- | --------------------------------------------------------- |
| `spec.pollInterval`               | duration    | How often to reconcile and refresh service account tokens |
| `spec.availableImages`            | []Image     | Allowed Velero and plugin images with their versions      |
| `spec.availableImages[].name`     | string      | Image identifier (`velero` or plugin name)                |
| `spec.availableImages[].versions` | []string    | Allowed versions for this image                           |
| `spec.availableImages[].image`    | string      | Container image reference                                 |
| `spec.imagePullSecrets`           | []SecretRef | Secrets for private registry authentication               |

> ⚠️ **Note:** Only one ProviderConfig may exist per Velero service provider instance, and its name must match the service provider's name.

## 🔐 Air-Gapped Environments

For air-gapped or enterprise environments, configure private registries via ProviderConfig:

```yaml
apiVersion: velero.services.openmcp.cloud/v1alpha1
kind: ProviderConfig
metadata:
  name: velero
spec:
  pollInterval: 15m
  availableImages:
    - name: velero
      versions: ["v1.17.2"]
      image: "harbor.internal/velero/velero"
    - name: aws
      versions: ["v1.13.2"]
      image: "harbor.internal/velero/velero-plugin-for-aws"
  imagePullSecrets:
    - name: harbor-credentials
```

## 🔧 Development Tasks

| Command                     | Description                              |
| --------------------------- | ---------------------------------------- |
| `task build`                | Build the binary                         |
| `task build:img:build-test` | Build the container image                |
| `task test`                 | Run unit tests                           |
| `task test-e2e`             | Run end-to-end tests                     |
| `task generate`             | Generate CRDs and code after API changes |
| `task validate`             | Run linters and formatters               |

## 📚 Additional Resources

- [Velero Documentation](https://velero.io/docs/)
- [Velero API Types](https://velero.io/docs/main/api-types/)
- [openMCP Project](https://github.com/openmcp-project)

## 🤝 Support, Feedback, Contributing

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/openmcp-project/service-provider-velero/issues). Contribution and feedback are encouraged and always welcome. For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## 🔒 Security / Disclosure

If you find any bug that may be a security problem, please follow our instructions at [in our security policy](https://github.com/openmcp-project/service-provider-velero/security/policy) on how to report it. Please do not create GitHub issues for security-related doubts or problems.

## 📜 Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/openmcp-project/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## 📄 Licensing

Copyright OpenControlPlane contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/openmcp-project/service-provider-velero).
---

<p align="center">
  <a href="https://apeirora.eu/content/projects/">
    <img alt="BMWK-EU funding logo" src="https://apeirora.eu/assets/img/BMWK-EU.png" width="300"/>
  </a>
</p>

<p align="center">
  OpenControlPlane is part of <a href="https://apeirora.eu/content/projects/">ApeiroRA</a>, an EU Important Project of Common European Interest (IPCEI-CIS).
</p>

<p align="center">
  Copyright Linux Foundation Europe. For web site terms of use, trademark policy and other project policies please see <a href="https://linuxfoundation.eu/en/policies">https://linuxfoundation.eu/en/policies</a>.
</p>
