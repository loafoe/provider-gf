# provider-gf

`provider-gf` is a [Crossplane](https://crossplane.io/) Provider for [Grafana](https://grafana.com/).

This provider is built for **Crossplane v2** and uses namespaced resources only.

## Supported Resources

- **Dashboard** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage Grafana dashboards with full JSON configuration and folder placement
- **DataSource** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage Grafana data sources (Prometheus, Loki, Tempo, etc.) with authentication options
- **DashboardPermission** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage complete permission sets for dashboards (per user/role/team with View/Edit/Admin levels)
- **Folder** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage Grafana folders with hierarchical structure support
- **LibraryPanel** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage Grafana library panels for reusable panel templates
- **Config** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage Grafana provider configuration (organization ID, default folder, timezone, theme)

## Installation

Install the provider into your Crossplane cluster:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-gf
spec:
  package: crossplane/provider-gf:latest
```

## Authentication

The provider supports two authentication methods. Both use namespaced `ProviderConfig` resources.

### Basic Auth

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: grafana-credentials
  namespace: default
type: Opaque
stringData:
  username: admin
  password: admin

---
apiVersion: gf.m.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: grafana-basic-auth
  namespace: default
spec:
  url: http://localhost:3000
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

### Service Account Token

```yaml
apiVersion: gf.m.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: grafana-basic-auth
  namespace: default
spec:
  url: http://localhost:3000
  credentials:
    source: Secret
    authType: token
    tokenAuth:
      tokenSecretRef:
        namespace: default
        name: grafana-token
        key: token
```

## Usage

### Creating a Dashboard

```yaml
apiVersion: oss.gf.m.crossplane.io/v1alpha1
kind: Dashboard
metadata:
  name: example-dashboard
  namespace: default
spec:
  providerConfigRef:
    name: grafana-basic-auth
  forProvider:
    configJson: |
      {
        "title": "Example Dashboard",
        "tags": ["crossplane", "example"],
        "timezone": "browser",
        "schemaVersion": 16,
        "version": 0,
        "refresh": "25s",
        "panels": [
          {
            "id": 1,
            "gridPos": {
              "h": 8,
              "w": 12,
              "x": 0,
              "y": 0
            },
            "type": "text",
            "title": "Welcome",
            "options": {
              "content": "# Hello from Crossplane!\n\nThis dashboard was created by the Grafana Crossplane provider.",
              "mode": "markdown"
            }
          }
        ]
      }
    folder: ""
    message: "Created by Crossplane"
    overwrite: true
```

### Dashboard Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `configJson` | string | Yes | The complete dashboard model JSON |
| `folder` | string | No | The UID of the folder to save the dashboard in |
| `message` | string | No | Commit message for version history |
| `orgId` | string | No | Organization ID (uses provider config default if not set) |
| `overwrite` | boolean | No | Overwrite existing dashboard with same UID/title (default: false) |

### Dashboard Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `configJson` | string | The observed dashboard model JSON |
| `dashboardId` | int64 | The numeric ID computed by Grafana |
| `folder` | string | The folder UID containing the dashboard |
| `uid` | string | The unique identifier used in URLs |
| `url` | string | The full URL of the dashboard |
| `version` | int64 | Version number, incremented on each save |

### Creating a Folder

```yaml
apiVersion: oss.gf.m.crossplane.io/v1alpha1
kind: Folder
metadata:
  name: monitoring-folder
  namespace: default
spec:
  providerConfigRef:
    name: grafana-basic-auth
  forProvider:
    title: "Monitoring"
    uid: "monitoring"
    orgRef:
      name: default-org
  write: true
  overwrite: true
```

### Creating a DataSource (Prometheus)

```yaml
apiVersion: oss.gf.m.crossplane.io/v1alpha1
kind: DataSource
metadata:
  name: prometheus-datasource
  namespace: default
spec:
  providerConfigRef:
    name: grafana-basic-auth
  forProvider:
    name: "Prometheus"
    type: "prometheus"
    url: "http://prometheus:9090"
    accessMode: "proxy"
    jsonDataEncoded: '{"httpMethod": "POST", "maxDataPoints": 435}'
    secureJsonDataEncodedSecretRef:
      namespace: default
      name: prometheus-credentials
      key: jsonDataEncoded
    orgRef:
      name: default-org
  write: true
  overwrite: true
```

### Creating a Library Panel

```yaml
apiVersion: oss.gf.m.crossplane.io/v1alpha1
kind: LibraryPanel
metadata:
  name: custom-metrics-panel
  namespace: default
spec:
  providerConfigRef:
    name: grafana-basic-auth
  forProvider:
    name: "Custom Metrics Panel"
    modelJson: |
      {
        "gridPos": {"x": 0, "y": 0, "w": 6, "h": 8},
        "id": 1,
        "title": "Custom Metrics",
        "type": "graph",
        "datasource": {"type": "prometheus", "uid": "prometheus"},
        "fieldConfig": {},
        "options": {},
        "targets": [
          {"refId": "A", "expr": "up", "legendFormat": "up"}
        ]
      }
    folderUid: "general"
    orgRef:
      name: default-org
  write: true
  overwrite: true
```

### Managing Dashboard Permissions

```yaml
apiVersion: oss.gf.m.crossplane.io/v1alpha1
kind: DashboardPermission
metadata:
  name: admin-perms
  namespace: default
spec:
  providerConfigRef:
    name: grafana-basic-auth
  forProvider:
    dashboardUid: "my-dashboard"
    orgRef:
      name: default-org
    permissions:
      - permission: "Admin"
        role: "Admin"
      - permission: "Edit"
        role: "Editor"
      - permission: "View"
        teamId: 12345
  write: true
  overwrite: true
```

### Config Resource Example

```yaml
apiVersion: oss.gf.m.crossplane.io/v1alpha1
kind: Config
metadata:
  name: grafana-config
  namespace: default
spec:
  providerConfigRef:
    name: grafana-basic-auth
  forProvider:
    orgId: 1
    defaultFolder: "general"
    timezone: "utc"
    theme: "dark"
  write: true
```

## Developing

1. Run `make submodules` to initialize the "build" Make submodule.
2. Run `make generate` to generate code.
3. Run `make reviewable` to run code generation, linters, and tests.
4. Run `make build` to build the provider.

### Running Locally

```shell
make run
```

### Running in a Kind Cluster

```shell
make dev
```

## License

Apache 2.0 - See [LICENSE](LICENSE) for more information.
