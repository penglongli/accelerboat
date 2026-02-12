# AccelerBoat：原理与架构

## 概述

**AccelerBoat** 是一款面向 Kubernetes 与局域网环境的 OCI 镜像仓库加速器。它加速集群内镜像拉取、减轻中心仓库压力，并提供统一出口与多账号访问能力。

### 目标

- **加速镜像拉取**：在大规模或跨地域集群中提升拉取速度。
- **复用已有 OCI 镜像**：发现并复用其他节点上已存在的镜像层（Docker/containerd），避免重复从上游仓库拉取。
- **收敛拉取流量**：统一处理认证、Manifest 与 Layer 请求，减少对中心仓库的重复请求。
- **统一出口**：所有节点通过同一代理出口访问仓库（适用于需固定 IP 白名单等场景）。
- **多账号支持**：一个代理域名下支持同一仓库的多个用户名/密码（如多区域、多团队）。

---

## 整体架构

AccelerBoat 以 HTTP/HTTPS 服务形式运行，通常在 Kubernetes 中多副本部署。它作为 **Registry 代理**：客户端（Docker、containerd 等）将镜像仓库地址配置为代理的 Host 和端口，由代理转发或加速请求。

代理主要做三件事：

1. 拦截 OCI Distribution 协议请求（认证、Manifest、Blob）。
2. 在本地或集群内已有缓存时直接响应。
3. 无缓存时回源到上游仓库，并可选择通过配置的 HTTP 代理出口。

### 核心组件

| 组件 | 作用 |
|------|------|
| **HTTP/HTTPS 服务** | 接收客户端请求；按 Host 匹配到对应仓库映射；按请求类型（认证、Manifest、Blob）分发。 |
| **Registry 代理（upstream proxy）** | 按代理 Host 封装的处理器：将请求改写为原始仓库地址，处理认证/Manifest/Blob 逻辑，并与「Master」协作。 |
| **Custom API（Master）** | 集中处理 Service Token、Manifest 以及 **Layer 位置解析**；仅当前「Master」节点执行，其他节点通过 HTTP 调用。 |
| **Master 选举** | 基于 Kubernetes Endpoints（及可选偏好配置）选出一个副本为 Master（如按 endpoint 字符串或固定 masterIP）。 |
| **Redis** | 共享的 Layer 位置索引：记录某 Layer 在哪些节点、以何种形式存在（静态文件或 OCI/containerd），供任意节点查询。 |
| **OCI 扫描器** | 定期扫描本机 containerd 内容存储，将 digest → 路径上报到 Redis。 |
| **静态文件监听** | 监听本地 Layer 目录；文件增删时更新 Redis，使新下载的 Layer 可被集群发现。 |
| **BitTorrent 处理** | 对超过大小阈值的 Layer 生成并参与种子分发，通过 P2P（UDP）在节点间传输，加速大文件分发。 |

---

## 请求处理流程（代理如何工作）

客户端请求会带上 **代理 Host**（如 `proxy.example.com`）。服务端根据该 Host 找到对应的 **Registry 映射**，得到 **原始仓库**（如 `registry-1.docker.io`），再对请求分类处理。

### 1. 请求分类

根据 URL 路径与方法区分：

- **Service Token**：`GET /service/token?service=...&scope=...`（获取仓库 OAuth2 Token）。
- **Head Manifest**：`HEAD /v2/<repo>/manifests/<tag>`（获取镜像 digest/头信息）。
- **Get Manifest**：`GET /v2/<repo>/manifests/<tag>`（获取 Manifest JSON）。
- **Get Blob**：`GET /v2/<repo>/blobs/sha256:<digest>`（下载某一层）。

其余请求通过反向代理直接转发到上游仓库。

### 2. Service Token（认证）

- 代理节点 **不直接** 向上游仓库请求 Token，而是将请求转给 **Master**（Custom API：`POST /customapi/service-token`）。
- Master 端：
  - 若有缓存（按 originalHost + service + scope 去重），直接返回。
  - 否则请求原始仓库的 Token 地址（可带上客户端 Authorization）。
  - 若配置了 **多用户**（`RegistryMapping.Users`），在首次失败或未带客户端认证时可依次用配置的用户名密码重试，成功后将 Token 缓存并返回。
- 返回给客户端的 `Www-Authenticate` 中的 realm 会被改写为代理的 HTTPS 地址，保证后续认证仍走代理。

**效果**：认证流量收敛到 Master；同一代理域名可支持多账号。

### 3. Head Manifest / Get Manifest

- 两种请求都转发到 Master（`POST /customapi/head-manifest`、`POST /customapi/get-manifest`）。
- Master 向 **原始仓库** 发起请求（携带客户端认证头），对结果做短期缓存（如 10 秒），再将头信息或 Manifest 内容返回给代理。
- 代理原样返回给客户端。

**效果**：Manifest 请求收敛并在 Master 侧缓存，减少对上游仓库的重复访问。

### 4. Get Blob（Layer 下载）

这里是加速与收敛的核心逻辑。

#### 步骤 A：本地检查

- 代理先检查该 Layer **是否已在本机** 存在，查找路径包括：
  - 普通下载完成目录（transfer path）
  - 小文件目录（Master 拉取的小 Layer）
  - OCI 缓存目录（containerd 导出等）
- 若找到，直接以文件形式响应客户端（支持 Range），**不再** 访问上游或 Master。

#### 步骤 B：向 Master 查询 Layer 位置

- 若本地没有，代理向 Master 请求：`POST /customapi/get-layer-info`，带上原始 Host、仓库、digest 及客户端头。
- Master 端逻辑：
  1. 向原始仓库发 **HEAD** 请求得到该 Layer 的 **Content-Length**。
  2. **查 Redis** 中该 digest 的索引：
     - **静态 Layer**：各节点上已落盘的 Layer（由静态文件监听或下载完成时上报）。
     - **OCI Layer**：各节点 containerd 中存在的 Layer（由 OCI 扫描器上报）。
  3. 对每个候选节点，Master 调用该节点的：
     - `GET /customapi/check-static-layer` 或 `GET /customapi/check-oci-layer`，校验文件存在且大小一致；若开启 BitTorrent 且 Layer 足够大，节点可返回 **torrent（base64）**。
  4. 若某节点校验通过，Master 向请求方返回 `{ located, filePath, fileSize, torrentBase64? }`。
  5. 若 **没有任何节点** 拥有该 Layer：
     - **小 Layer**（如 &lt; 20MB）：由 Master **自己** 从原始仓库下载到小文件目录，并把自己的地址作为 `located` 返回。
     - **大 Layer**：Master **分发任务**：选一个工作节点（如按当前任务数负载均衡），调用该节点的 `GET /customapi/download-layer`。该节点从原始仓库下载到 transfer 目录，可选生成 torrent，返回 `located`、路径及可选的 `torrentBase64`。

#### 步骤 C：请求方拉取 Layer

- 代理（请求方）拿到 `located`、`filePath` 及可选的 `torrentBase64`。
- 若 Layer 已在 **本机**（例如刚在本机下载完成或二次本地检查命中），则直接从本地文件响应客户端。
- 否则从其他节点 **拉取**：
  - 若有 **torrent** 且开启 BitTorrent：用 BitTorrent 客户端拉取到 torrent 目录，再拷贝到目标路径（如 transfer）。
  - 否则 **TCP 直传**：`GET http://<located>:<httpPort>/customapi/transfer-layer-tcp?file=<filePath>`，从拥有该文件的节点流式下载。
- 落盘后，代理从本地存储把 Blob 响应给客户端。

**效果**：Layer 在集群内可被发现（Redis + OCI 扫描 + 静态监听）；同一 digest 通常只由某一节点从上游拉取一次，其余节点从该节点经 TCP 或 BitTorrent 获取，实现流量收敛与加速。

---

## 镜像发现（为何能复用已有镜像）

### OCI（containerd）发现

- 若开启 **containerd**，每个节点会运行 **OCI 扫描器**：
  - 连接本机 containerd 套接字（如 `/run/containerd/containerd.sock`）。
  - 遍历 `k8s.io` 命名空间下的 content store，收集 Layer digest。
  - 定时（如每 60 秒）将 **digest → 本节点 + 类型（CONTAINERD）** 写入 **Redis**。
- Master 在解析 Layer 位置时，会从 Redis 拿到 OCI 类型记录，再请求对应节点执行 **check-oci-layer**。节点通过 containerd 的 content API 提供该 digest 的访问（或导出）；Master 再告诉请求方从该节点拉取（当前为 TCP；OCI 层未来也可走 BitTorrent）。

因此，只要 **任意** 节点曾拉过该镜像（如通过 containerd），其 Layer 就会进入集群索引，可被复用。

### 静态 Layer 发现

- 当某 Layer 被代理 **下载完成**（落入 transfer、小文件或 OCI 路径）时，该节点的 **静态文件监听** 发现新的 `.tar.gzip` 文件，并调用 **Redis** 登记「digest → 本节点 + 路径」。
- 这样每次下载完成都会自动成为后续可被发现的「静态」Layer。

两者结合：**只要 Layer 在集群内任一节点存在（无论是历史拉取还是 containerd 已有），代理都可以在不再次访问上游仓库的情况下提供该 Layer。**

---

## BitTorrent（P2P）加速

- 对 **超过配置大小阈值** 的 Layer，可生成 **种子（torrent metainfo）**，通过 BitTorrent 协议（UDP）在节点间分发。
- 当某节点从仓库下载完 Layer（或已在静态/OCI 缓存中）时，可调用 **BitTorrent 处理逻辑** 为该文件生成种子，并在 Layer 信息响应中返回 `torrentBase64`。
- 收到 `torrentBase64` 的请求方可以 **通过 BitTorrent 下载**（多源、UDP），而不是只从单一节点 HTTP 拉取，在集群规模较大时可明显加快分发。
- 种子与数据存放在专用 torrent 目录；上传/下载限速、Announce 地址等可配置。

---

## 统一流量出口（Egress 代理）

- 在受限网络中，上游仓库可能只允许 **固定 IP 白名单**。
- AccelerBoat 支持配置 **HTTP 代理**（`ExternalConfig.HTTPProxyUrl`）。所有访问 **原始仓库** 的出站请求（Token、Manifest、Layer）都经该代理发出。
- 若所有副本使用同一代理，则对仓库而言 **全部** 拉取流量都来自该代理出口 IP，只需对该 IP 做白名单即可。

---

## 同仓库多用户/多密码

- 一个代理 Host（如 `proxy.example.com`）可对应 **一个** 原始仓库，但支持 **多组** 账号。
- 在仓库映射中可配置 **Users**（用户名 + 密码列表）。
- Master 在获取 **Service Token** 时，若客户端自带认证失败或未带认证，会依次用配置的 **Users** 重试，直到某组成功；该 Token 被缓存并返回。
- 这样同一域名可面向多套仓库身份（如不同区域、不同团队），而客户端无需关心具体用哪组账号；由服务端自动选用可用账号。

---

## Master 选举

- 通过 **Kubernetes 该服务的 Endpoints** 获取所有副本（Pod IP + 端口）。
- **Master** 的选取规则：
  - 若配置了 **masterIP**：endpoint 中与该 IP 一致的即为 Master。
  - 否则：在 endpoint 列表（可按 **prefer nodes** 标签过滤）中，取 **字符串最大** 的 endpoint（如按 ASCII 比较）作为 Master，保证在副本集不变时 Master 稳定。
- 只有 **Master** 处理需要协调的 Custom API（Service Token、Manifest、get-layer-info、分发下载）；其他节点作为「请求方」以及本机静态/OCI Layer 与 TCP 传输的提供方。

---

## 存储与路径

- **Redis**：Layer 索引（digest → (节点, 路径, 类型, 时间戳) 等），用于发现与清理；多节点加速时通常必选。
- **本地路径**（可配置）：
  - **Download 路径**：临时下载，不保证完整。
  - **Transfer 路径**：普通（TCP）下载完成且完整性有保证的 Layer。
  - **Small 路径**：Master 拉取的小 Layer（如 &lt; 20MB）。
  - **Torrent 路径**：BitTorrent 下载与做种存储。
  - **OCI 路径**：从 containerd 导出的缓存 Layer。
- **Event 文件**：可选，用于记录结构化事件（如 get-layer-info、download-layer、错误等），便于审计与排障。

---

## 小结

AccelerBoat 作为 **OCI Registry 代理**，实现了：

1. **流量收敛**：认证、Manifest、Blob 请求经单一 Master 与共享 Redis 索引协调。
2. **Layer 发现**：通过 containerd（OCI）与静态文件监听，复用集群内已有 Layer，避免重复从上游拉取。
3. **任务分发**：大 Layer 由 Master 分配给工作节点拉取，并可选 **BitTorrent** 做 P2P 传输。
4. **统一出口**：通过 HTTP 代理访问上游，满足白名单等网络限制。
5. **多账号**：同一代理域名下支持多组仓库凭证。

综合这些机制，达到加速镜像拉取、减轻中心仓库压力与带宽占用，并适应受限网络与多账号场景的目的。
