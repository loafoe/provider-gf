## Overview

This provider allows platform teams to embed Grafana observability management into their control planes, managing dashboards, data sources, folders, and alerting configuration alongside infrastructure and application deployments.

## Prerequisites

The provider requires access to a Grafana instance with API access enabled. Authentication can be configured using either:

- **Basic Auth**: Username and password (typically the admin user)
- **Service Account Token**: A Grafana service account token with appropriate permissions

## Installation

Install the provider by applying the following `Provider` manifest to your Crossplane cluster:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-gf
spec:
  package: xpkg.upbound.io/loafoe/provider-gf:v0.1.0
```

## Configuration

1. **Create a Secret** containing the credentials to connect to your Grafana instance.
2. **Apply a `ProviderConfig`** to configure the connection.

### Basic Auth Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: grafana-credentials
  namespace: default
type: Opaque
stringData:
  username: admin
  password: your-password
---
apiVersion: gf.m.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  namespace: default
spec:
  url: https://grafana.example.com
  credentials:
    source: Secret
    authType: basic
    basicAuth:
      usernameSecretRef:
        namespace: default
        name: grafana-credentials
        key: username
      passwordSecretRef:
        namespace: default
        name: grafana-credentials
        key: password
```

### Service Account Token Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: grafana-token
  namespace: default
type: Opaque
stringData:
  token: glsa_xxxxxxxxxxxxxxxxxxxxx
---
apiVersion: gf.m.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  namespace: default
spec:
  url: https://grafana.example.com
  credentials:
    source: Secret
    authType: token
    tokenAuth:
      tokenSecretRef:
        namespace: default
        name: grafana-token
        key: token
```

## Supported Resources

### OSS Resources (`oss.gf.m.crossplane.io/v1alpha1`)

- **Dashboard** - Manage Grafana dashboards
- **DataSource** - Configure data sources (Prometheus, Loki, etc.)
- **Folder** - Organize dashboards into folders
- **Organization** - Manage Grafana organizations
- **LibraryPanel** - Create reusable library panels
- **DashboardPermission** - Control dashboard access permissions
- **FolderPermission** - Control folder access permissions

### Alerting Resources (`alerting.gf.m.crossplane.io/v1alpha1`)

- **ContactPoint** - Configure alerting notification channels

## License

This project is licensed under the Apache 2.0 License.
