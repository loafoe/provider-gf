# provider-gf

`provider-gf` is a [Crossplane](https://crossplane.io/) Provider for [Grafana](https://grafana.com/).

This provider is built for **Crossplane v2** and uses namespaced resources only.

## Supported Resources

- **Dashboard** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage Grafana dashboards with full JSON configuration and folder placement
- **DataSource** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage Grafana data sources (Prometheus, Loki, Tempo, etc.) with authentication options
- **DashboardPermission** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage complete permission sets for dashboards (per user/role/team with View/Edit/Admin levels)
- **Folder** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage Grafana folders with hierarchical structure support
- **LibraryPanel** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage Grafana library panels for reusable panel templates
- **Team** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage Grafana teams, optionally owning their membership
- **TeamMember** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage a single user's membership of a team
- **TeamPreference** (`oss.gf.m.crossplane.io/v1alpha1`) - Manage a team's theme, timezone and home dashboard
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
        teamRef:
          name: platform-team
  write: true
  overwrite: true
```

Permissions that aren't listed are removed — the resource owns the whole set.

### Granting Access to Teams

Both `DashboardPermission` and `FolderPermission` can grant access to a team.
Reference it rather than hardcoding its ID, so the grant follows the team if it
is recreated with a new ID:

```yaml
permissions:
  - teamRef:
      name: platform-team        # by name
    permission: Edit
  - teamSelector:
      matchLabels:
        grafana.example.com/team: security
    permission: View
```

A literal `teamId` still works and takes precedence over a reference, but
nothing checks that it still refers to the team you meant — if the team is
deleted and a new one reuses the ID, the grant silently moves.

### Creating a Team

```yaml
apiVersion: oss.gf.m.crossplane.io/v1alpha1
kind: Team
metadata:
  name: platform-team
  namespace: default
spec:
  providerConfigRef:
    name: grafana-basic-auth
  forProvider:
    name: "Platform"
    email: "platform@example.com"
    # Optional. Users must already exist in Grafana.
    admins:
      - platform-lead@example.com
    members:
      - sre1@example.com
      - sre2@example.com
    orgRef:
      name: default-org
```

### Managing Team Membership

A `Team` only owns its membership when `members` or `admins` is set. When
either is set, the whole membership set is authoritative and anyone not listed
is removed from the team. Leave both unset to manage membership with
standalone `TeamMember` resources instead - don't set both, or the two
resources will fight over the same membership.

```yaml
apiVersion: oss.gf.m.crossplane.io/v1alpha1
kind: TeamMember
metadata:
  name: platform-sre-oncall
  namespace: default
spec:
  providerConfigRef:
    name: grafana-basic-auth
  forProvider:
    teamRef:
      name: platform-team
    # userId is preferred over loginOrEmail because it is stable.
    loginOrEmail: sre-oncall@example.com
```

`loginOrEmail` is matched case-insensitively against the login and email of the
users in the resource's organization. Grafana refuses to add a user to a team
unless they already belong to the organization, so the user must exist there.

### Configuring Team Preferences

```yaml
apiVersion: oss.gf.m.crossplane.io/v1alpha1
kind: TeamPreference
metadata:
  name: platform-team-preferences
  namespace: default
spec:
  providerConfigRef:
    name: grafana-basic-auth
  forProvider:
    teamRef:
      name: platform-team
    theme: dark
    timezone: utc
    homeDashboardUid: "platform-overview"
```

Grafana replaces the whole preference object on update, so any field you omit
is reset to the Grafana default. Use `""` to explicitly select the default
(e.g. no home dashboard).

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

### Testing Against a Live Grafana

Most behaviour only shows up against a real Grafana — how it handles a team
that is renamed in the UI, a member added out of band, a preference object it
returns as `{}` rather than zero values. The live tests drive the real
controllers through that.

They are skipped by `make test` and need a URL and token:

```shell
make test-live \
  GRAFANA_URL=https://grafana.example.com \
  GRAFANA_TOKEN=glsa_... \
  GRAFANA_ORG_ID=1
```

Point these at a throwaway instance or an org you don't mind mutating: the
tests create and delete teams, and add and remove real users from them. Every
team they create is deleted afterwards, including when a test fails.

## License

Apache 2.0 - See [LICENSE](LICENSE) for more information.
