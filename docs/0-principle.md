# AccelerBoat: Principles and Architecture

## Overview

**AccelerBoat** is an OCI image registry accelerator for Kubernetes and LAN environments. It speeds up image pulls across the cluster, reduces load on the central registry, and provides unified egress and multi-account support for registry access.

### Goals

- **Speed up image pulls** in large-scale or cross-region clusters.
- **Reuse existing OCI images** already present on other nodes (Docker/containerd) so the same layer is not pulled again from the upstream registry.
- **Converge pull traffic** so that authentication, manifest, and layer requests are coordinated and duplicated pulls are avoided.
- **Unify egress** so all nodes pull through a single proxy (e.g. for firewall/whitelist scenarios).
- **Support multiple credentials** per registry via one proxy domain (e.g. multiple users/passwords for the same registry).

---

## High-Level Architecture

AccelerBoat runs as an HTTP/HTTPS service, typically deployed in Kubernetes with multiple replicas. It behaves as a **registry proxy**: clients (Docker, containerd, etc.) point their registry configuration to the proxy host and port instead of the upstream registry. The proxy then:

1. Intercepts OCI Distribution API requests (auth, manifest, blob).
2. Serves them from local or cluster-wide caches when possible.
3. Falls back to the upstream registry and optionally exports traffic through a configurable HTTP proxy.

### Main Components

| Component | Role |
|-----------|------|
| **HTTP/HTTPS server** | Accepts client requests; routes by `Host` to the matching registry mapping; dispatches by request type (auth, manifest, blob). |
| **Registry proxy (upstream proxy)** | Per proxy-host handler that rewrites URLs to the original registry, handles auth/manifest/blob logic, and talks to the “master” for coordination. |
| **Custom API (master)** | Centralized logic for service token, manifest, and **layer location**. Only the current “master” node runs this logic; other nodes call it over HTTP. |
| **Master election** | Uses Kubernetes Endpoints (and optional prefer config) to pick one replica as “master” (e.g. by endpoint string or fixed `masterIP`). |
| **Redis** | Shared store for layer location: which node has which layer (static files or OCI/containerd), so any node can discover existing layers in the cluster. |
| **OCI scanner** | Periodically scans containerd content store on each node and reports layer digest → path to Redis. |
| **Static file watcher** | Watches local layer directories; on file create/remove, updates Redis so newly downloaded layers are discoverable cluster-wide. |
| **BitTorrent handler** | For layers above a size threshold, can generate and serve torrents so layers are distributed via P2P (UDP) instead of single-node TCP. |

---

## Request Flow (How the Proxy Works)

Client requests hit the proxy with a **proxy host** (e.g. `proxy.example.com`). The server resolves the **registry mapping** for that host to get the **original registry** (e.g. `registry-1.docker.io`) and then classifies the request.

### 1. Request classification

Requests are classified using URL path and method:

- **Service token**: `GET /service/token?service=...&scope=...` (OAuth2 token for registry).
- **Head manifest**: `HEAD /v2/<repo>/manifests/<tag>` (get image digest/headers).
- **Get manifest**: `GET /v2/<repo>/manifests/<tag>` (get manifest JSON).
- **Get blob**: `GET /v2/<repo>/blobs/sha256:<digest>` (download a layer).

All other requests are forwarded to the upstream registry via a reverse proxy.

### 2. Service token (authentication)

- The proxy node does **not** call the upstream registry directly for token.
- It forwards the token request to the **master** (Custom API: `POST /customapi/service-token`).
- The master:
  - May return a cached token (keyed by original host + service + scope).
  - Otherwise calls the original registry’s token URL (using client `Authorization` if present).
  - If the registry supports it, the master can retry with **multiple configured users** (`RegistryMapping.Users`); the first successful token is cached and returned.
- The response’s `Www-Authenticate` realm is rewritten to point to the proxy’s HTTPS endpoint so subsequent auth still goes through the proxy.

**Effect**: Authentication traffic is converged through the master; multiple accounts per registry are supported with a single proxy domain.

### 3. Head manifest / Get manifest

- Both are forwarded to the master (`POST /customapi/head-manifest` and `POST /customapi/get-manifest`).
- The master calls the **original registry** (with the client’s auth headers), caches the result briefly (e.g. 10s), and returns headers or manifest body to the proxy.
- The proxy returns that to the client.

**Effect**: Manifest traffic is converged and cached on the master, reducing repeated hits to the upstream registry.

### 4. Get blob (layer download)

This is where most of the acceleration and convergence logic lives.

#### Step A: Local check

- The proxy first checks if the layer **already exists on the current node** in:
  - Transfer path (completed normal downloads)
  - Small-file path (small layers stored by master)
  - OCI path (containerd cache)
- If found, the proxy serves the file directly to the client (with optional range support) and **does not** call the upstream registry or the master.

#### Step B: Ask master for layer location

- If not found locally, the proxy calls the master: `POST /customapi/get-layer-info` with original host, repo, digest, and client headers.
- The master:
  1. Gets the layer’s **expected content length** from the upstream registry (HEAD).
  2. **Queries Redis** for this digest:
     - **Static layers**: Nodes that have already stored this layer under transfer/small/OCI paths (reported by the static watcher or by download completion).
     - **OCI layers**: Nodes that have this layer in containerd (reported by the OCI scanner).
  3. For each candidate node, the master calls that node:
     - `GET /customapi/check-static-layer` or `GET /customapi/check-oci-layer` to verify the file exists and size matches; the node may return a **torrent** (base64) if BitTorrent is enabled and the layer is large enough.
  4. If a valid location is found, the master returns `{ located, filePath, fileSize, torrentBase64? }` to the requester.
  5. If **no** node has the layer:
     - For **small layers** (e.g. &lt; 20MB): the master downloads the layer from the **original registry** itself, saves to small-file path, and returns its own address as `located`.
     - For **large layers**: the master **distributes** the task: it picks a worker node (e.g. by current task count), calls that node’s `GET /customapi/download-layer`. That node downloads from the original registry, saves to transfer path, optionally generates a torrent, and returns `located` + path + optional `torrentBase64`.

#### Step C: Requester downloads the layer

- The proxy (requester) receives `located`, `filePath`, and optionally `torrentBase64`.
- If the layer is **on the current node** (e.g. just downloaded locally or found in a retry), it serves from local and returns.
- Otherwise it **downloads** the layer:
  - If **torrent** is provided and BitTorrent is enabled: use the BitTorrent client to download the layer (P2P) into the torrent path, then copy to the target path (e.g. transfer path).
  - Else: **TCP transfer** — `GET http://<located>:<httpPort>/customapi/transfer-layer-tcp?file=<filePath>` to stream the file from the node that has it.
- After the file is saved locally, the proxy serves the blob to the client from local storage.

**Effect**: Layers are discovered cluster-wide (Redis + OCI scanner + static watcher); only one node typically pulls from the upstream for a given digest, and other nodes get the layer via TCP or BitTorrent from that node. Pull traffic is converged and accelerated.

---

## Image Discovery (Why Existing Images Are Reused)

### OCI (containerd) discovery

- If **containerd** is enabled, each node runs an **OCI scanner** that:
  - Connects to the local containerd socket (`/run/containerd/containerd.sock`).
  - Walks the `k8s.io` namespace content store and collects layer digests.
  - Periodically (e.g. every 60s) **reports** `digest → node + type (CONTAINERD)` to **Redis**.
- When the master looks up a layer, it gets OCI-layer entries from Redis and asks those nodes to **check OCI layer**. The node uses containerd’s content API to serve (or export) that digest; the master then tells the requester to pull from that node (today via TCP; BitTorrent for OCI could be added later).

So if **any** node has already pulled an image (e.g. via containerd), that node’s layers are visible to the cluster and can be reused.

### Static layer discovery

- When a layer is **downloaded** by the proxy (to transfer path, small-file path, or OCI path), the **static file watcher** on that node sees the new `.tar.gzip` file and calls **Redis** to record `digest → this node + path`.
- So every completed download is automatically registered as a “static” layer for future lookups.

Together, OCI + static discovery mean: **if the layer exists anywhere in the cluster (either from a previous pull or from containerd), the proxy can serve it without pulling from the upstream registry again.**

---

## BitTorrent (P2P) Acceleration

- For layers **above a configurable size threshold**, the system can generate a **torrent** (metainfo) and distribute it via the BitTorrent protocol (UDP).
- When a node finishes downloading a layer from the registry (or has it in static/OCI cache), it can call the **BitTorrent handler** to generate a torrent for that file and return `torrentBase64` in the layer-info response.
- Requesters that receive `torrentBase64` can **download via BitTorrent** (multiple peers, UDP) instead of a single HTTP TCP stream from one node, which can speed up distribution in large clusters.
- Torrent files are stored under a dedicated torrent path; upload/download rate limits and announce URL are configurable.

---

## Unified Traffic Export (Egress Proxy)

- In restricted networks, the upstream registry may only allow a **whitelist of IPs**.
- AccelerBoat supports an **HTTP proxy** (`ExternalConfig.HTTPProxyUrl`). All outbound HTTP/HTTPS requests to the **original registry** (token, manifest, layer) use this proxy.
- If every AccelerBoat replica uses the same proxy, **all** pull traffic to the registry appears to come from the proxy’s egress IP, so only that IP needs to be whitelisted.

---

## Multiple Users/Passwords per Registry

- A single proxy host (e.g. `proxy.example.com`) can map to **one** original registry but support **multiple** accounts.
- In the registry mapping, you can configure **Users** (username + password list).
- When the master tries to get a **service token** and the client’s own credentials fail (or are not provided), it **retries** with each configured user until one succeeds; that token is cached and returned.
- So one domain can front multiple registry identities (e.g. for different teams or regions) without the client having to know which credentials to use; the server picks a valid one.

---

## Master Election

- AccelerBoat uses **Kubernetes Endpoints** for the AccelerBoat service to discover all replicas (pod IPs + port).
- **Master** is chosen by:
  - If **masterIP** is set: the endpoint that matches that IP is master.
  - Else: among endpoints (optionally filtered by **prefer nodes** label selector), the one with the **largest endpoint string** (e.g. ASCII comparison) is master. So the master is deterministic and stable as long as the set of endpoints doesn’t change.
- Only the **master** handles Custom API calls that need to coordinate (service token, manifest, get-layer-info, distribute download). Other nodes act as “requesters” and as servers for their own static/OCI layers and TCP transfer.

---

## Data Stores and Paths

- **Redis**: Layer index (digest → list of (node, path, type, timestamp)). Used for discovery and cleanup. Optional but required for multi-node acceleration.
- **Local paths** (configurable):
  - **Download path**: Temporary layer downloads; not guaranteed complete.
  - **Transfer path**: Completed layers from normal (TCP) download; integrity guaranteed.
  - **Small file path**: Small layers (e.g. &lt; 20MB) downloaded by the master.
  - **Torrent path**: BitTorrent download/cache and seed storage.
  - **OCI path**: Cached layers exported from containerd.
- **Event file**: Optional file sink for structured events (e.g. get-layer-info, download-layer, errors) for auditing or debugging.

---

## Summary

AccelerBoat is an **OCI registry proxy** that:

1. **Converges** auth, manifest, and blob traffic through a single logical “master” and shared Redis index.
2. **Discovers** existing layers on nodes via containerd (OCI) and static file watcher, so repeated pulls for the same digest are served from the cluster.
3. **Distributes** large layer downloads to worker nodes and optionally uses **BitTorrent** for P2P transfer.
4. **Unifies egress** via an HTTP proxy for whitelist-friendly access to the upstream registry.
5. **Supports multiple credentials** per registry under one proxy domain.

By combining these mechanisms, it speeds up image pulls, reduces load and bandwidth to the central registry, and adapts to restricted or multi-account environments.
