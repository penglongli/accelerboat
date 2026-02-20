# AccelerBoat: 原理与架构

## 概述

**AccelerBoat** 是一款面向 Kubernetes 与局域网环境的 OCI 镜像仓库加速器。它通过收敛请求（流量），并通过 TCP/BitTorrent 在局域网内进行流量分发来加速集群内镜像的拉取，减轻中心仓库的压力。

### 提供的能力

- **镜像发现**: 支持每个节点发现存在的 OCI 镜像，并分析镜像的 Layer 层。假如节点存在镜像，则不需要向中心仓库拉取；
- **流量收敛**: 由 Master 节点统一处理 Auth、Manifest 与 DownloadBlob 的请求，减少对中心仓库的重复请求，避免中心仓库出现 429 Too-Many-Requests 的错误；
- **镜像加速**: 通过 TCP/BitTorrent 协议，加快局域网内的镜像分发速度；
- **统一出口**: 所有节点通过同一代理出口（HTTP_PROXY）访问源仓库，适用于需要固定 IP 白名单的场景
- **多账户支持**: 一个代理域名下支持同一个仓库多个用户名/密码（适用于同一个代理域名，针对不同镜像仓库有多套用户名密码）

## 整体架构

AccelerBoat 以 HTTP/HTTPS 服务形式运行，通常在 Kubernetes 中多副本部署。它作为 Registry 的代理，支持客户端（Docker/Containerd 等）通过 Registry V2 协议进行拉取。

它通常支持两种模式：
1. RegistryMirror: 在 Docker/Containerd 配置中配置 mirrors（使用 http://localhost:2080 ）并进行重启
2. DomainProxy：在配置文件中配置镜像代理域名，并指定 Original Registry 域名的方式：
   - 增加一个自定义域名，并指向 127.0.0.1。如：accelerboat.image-proxy.com
   - 申请域名的 TLS 证书（cert/key）
   - 指定源镜像仓库的域名，如腾讯云镜像仓库：test-tcr.tencentcloudcr.com

针对第一种方式，用户需要侵入式修改主机的 Containerd/Docker 的配置。所以一般推荐第二种方式，配置示例：
```
registryMappings:
- enable: true
  originalHost: "test-tcr.tencentcloudcr.com"
  proxyHost: "accelerboat.image-proxy.com"
  proxyCert: "Base64(TLS-Cert)"
  proxyKey: "Base64(TLS-Key)"
```

### 工作流程

客户端在镜像拉取镜像的时候，请求会被转发到本机的 AccelerBoat 实例上，AccelerBoat 会根据请求的 Host 找到源仓库的映射，并对请求进行分类处理。

![arch-summary](images/arch-0-summary.png)

#### 本机请求分类

根据 URL 路径与方法区分：

- **Service Token**：`GET /service/token?service=...&scope=...`（获取仓库 OAuth2 Token）。
- **Head Manifest**：`HEAD /v2/<repo>/manifests/<tag>`（获取镜像 digest/头信息）。
- **Get Manifest**：`GET /v2/<repo>/manifests/<tag>`（获取 Manifest JSON）。
- **Get Blob**：`GET /v2/<repo>/blobs/sha256:<digest>`（下载某一层 Layer）。

在获取到请求之后，将上述请求转发给 Master 节点进行处理。其余请求通过反向代理直接转发到上游仓库。

#### Master 处理

Master 会提供 /customapi 来处理每个 AccelerBoat 实例转发过来的请求

**1. Service Token**

若有缓存（按 originalHost + service + scope 作为 cache key），直接返回。
- 否则请求原始仓库的 Token 地址（可带上客户端 Authorization）。
- 若配置了 **多用户**（`RegistryMapping.Users`），在首次失败或未带客户端认证时可依次用配置的用户名密码重试
- 成功后将 Token 缓存（缓存 10s）并返回。

**2. Head Manifest/Get Manifest**

Master 向 **原始仓库** 发起请求（携带客户端认证头），对结果做短期缓存（如 10 秒），再将头信息或 Manifest 内容返回给代理。

**效果**：Manifest 请求收敛并在 Master 侧缓存，减少对上游仓库的重复访问。

**3. Get Blob(Layer 下载)**

下载 Layer 的流程比较复杂，参考下述的([镜像 Layer 下载](#镜像-layer-下载))

###  镜像 Layer 下载

这里是整个镜像加速的核心逻辑：
1. AccelerBoat 实例接收到 GetBlob 的下载请求之后，优先检查本地是否存在 Layer 缓存
    - 如果存在，则直接 rewrite 给 OCI 层
2. 未找到后，向 Master 发送 /customapi/get-layer-info 请求，查询 Layer 的位置
    - 查询 Redis 中该 Layer 的索引：
      - 静态 Layer：各个节点上已经落盘的 Layer 文件
      - OCI Layer：各节点 Containerd 中存在的 Layer
    - 对从 Redis 中查找到的 Layer 进行校验，校验通过则直接返回
3. 如果未从 Redis 中找到局域网内存在这个 Layer
   - Master 节点将这个下载 Layer 的任务分配给空闲节点
   - 空闲节点进行 Layer 下载，下载完成后返回 Layer Located 给 Master
4. Master 将 Layer 的位置信息返回给客户端机器的 AccelerBoat，客户端机器的 AccelerBoat 实例从局域网内下载 Layer

![download-blob](images/arch-1-download-blob.png)

### 镜像发现机制

镜像发现的设计理念参考了开源组件 [spegel](https://github.com/spegel-org/spegel)

在 AccelerBoat 中通过 Containerd 的 OCI 接口，获取存储的 `OCI Image` 信息，并将每个节点的信息缓存到 Redis 中。

当 Master 节点接收到下载 Layer 的请求之后，会从 Redis 中查询是否在集群中有节点具备这个 Layer。

如果发现有节点具备这个 Layer，就不会回源进行下载。

![image-discovery](./images/arch-2-image-discovery.png)

### 统一流量出口

- 在受限网络中，上游仓库可能只允许 **固定 IP 白名单**。
- AccelerBoat 支持配置 **HTTP 代理**（`ExternalConfig.HTTPProxyUrl`）。所有访问 **原始仓库** 的出站请求（Token、Manifest、Layer）都经该代理发出。
- 若所有副本使用同一代理，则对仓库而言 **全部** 拉取流量都来自该代理出口 IP，只需对该 IP 做白名单即可。

![http-proxy](./images/arch-3-httpproxy.png)

### 同仓库/多用户名密码

- 一个代理 Host（如 `proxy.example.com`）可对应 **一个** 原始仓库，但支持 **多组** 账号。
- 在仓库映射中可配置 **Users**（用户名 + 密码列表）。
- Master 在获取 **Service Token** 时，若客户端自带认证失败或未带认证，会依次用配置的 **Users** 重试，直到某组成功；该 Token 被缓存并返回。
- 这样同一域名可面向多套仓库身份（如不同区域、不同团队），而客户端无需关心具体用哪组账号；由服务端自动选用可用账号。


## 高可用设计

在进行镜像拉取时我们做了分级传输的限制。BitTorrent 协议能够大幅度提高传输的速度，但是它需要一定的 CPU/内存占用，所以我们采用了根据镜像 Layer 大小的一个分级的措施：
- 超过 20M 的，内网进行镜像传输会走 TCP 下载的方式；
- 大于 10M 的会走 P2P 的 Torrent 协议进行传输。

并且我们会监控传输的过程，一旦传输出现问题（源镜像 Layer 被误删、传输中断、传输无速度等），会对镜像及时进行回源，使用源中心仓库进行下载，在最大程度上保证镜像拉取达到 100% 成功率。

![ha](./images/arch-4-ha.png)