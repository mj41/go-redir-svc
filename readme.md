# go-redir-svc

A lightweight Go service for handling domain redirects with custom JSONL logging and group-based certificate management.

## Architecture

- **Service**: A single Go application that handles all redirect traffic.
- **Configuration**: A single declarative YAML file defining redirect groups, domains, and targets.
- **Deployment**: A single `k8s.yaml` file containing all Kubernetes resources (Deployment, Service, PVC, ConfigMap, Certificates, HTTPRoutes).
- **Logging**: 
    - Custom JSONL format (including `timestamp`, `group`, `scheme`, `host`, `path`, `referer`, `user_agent`, `client_ip`, `target`).
    - **Per-group and per-day logging**: each group of sites logs to its own file for each UTC day,
      `<log_file without .jsonl>-<YYYY-MM-DD>-<host>.jsonl` (e.g. `shiftate-2026-10-09-go-redir-svc-7d9….jsonl`)
      in `-log-dir` (`/var/log/redirects`). A day's file is not written once the day is over, so anything
      may take it away; the host part keeps two pods from writing the same file.
    - In the cluster (mj41-linode `apps/base/go-redir-svc`): the log directory is local disk (an
      `emptyDir`), and a sidecar moves the finished days to Object Storage, encrypted.
- **TLS/DNS**: 
    - Domains are grouped to share `Certificate` objects (cert-manager).
    - `HTTPRoute` objects route traffic from the Gateway to this service.

## Declarative Definition (`config.yaml`)

The entire redirect logic is defined in one file. The service matches the incoming `Host` header against these groups.

```yaml
groups:
  example-com:
    log_file: "example-com.jsonl"
    target_base: "https://example.com"
    allowed_schemes: ["http", "https"]
    domains:
      - name: example.com
        allowed_schemes: ["http"] # Only handle HTTP -> HTTPS jump
      - name: www.example.com
      - name: old.example.com
      - name: legacy.example.com

  example-org:
    log_file: "example-org.jsonl"
    target_base: "https://example.org"
    allowed_schemes: ["http", "https"]
    domains:
      - name: example.org
        allowed_schemes: ["http"]
      - name: www.example.org
      - name: blog.example.org
      - name: www.blog.example.org
```

## Kubernetes Integration

### 1. Storage
Local disk for today's files; the finished days go to Object Storage (a sidecar in the cluster's
manifests). No volume.

### 2. Routing & TLS
For each group defined in the config, we maintain:
- A `Certificate` in `envoy-gateway-system` covering all domains in the group.
- An `HTTPRoute` that points all domains in the group to the `go-redir-svc`.

### 3. Logging Format
Example JSONL entry:

```json
{"ts":"2025-12-18T10:00:00Z", "group":"example-com", "scheme":"https", "host":"www.example.com", "path":"/old-page", "ip":"1.2.3.4", "referer":"https://google.com", "ua":"Mozilla...", "target":"https://example.com/old-page"}
```

## Best Practices

### HTTP to HTTPS
The global HTTP to HTTPS redirection should be handled at the **Gateway level** (e.g., Envoy Gateway `HTTPRoute` with a `RequestRedirect` filter on port 80). This ensures all traffic is encrypted before it even reaches `go-redir-svc`.

### `www.` Prefix for Real Servers
For a "real" server (e.g., `example.com` running a dedicated app), the best practice is to use `go-redir-svc` to handle the `www.example.com` -> `https://example.com` redirect.

**Why use `go-redir-svc` for this?**
1.  **Centralized Logging**: You get consistent JSONL logs for all entry-point redirects, including `www` variants of your main sites.
2.  **Clean Application Logic**: Your main application only needs to handle its canonical domain (`example.com`), keeping its configuration and code simpler.
3.  **Infrastructure Consistency**: All "alias" and "legacy" domain logic is consolidated in one place (`config.yaml`), rather than being scattered across multiple application deployments.
