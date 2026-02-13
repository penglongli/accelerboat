# AccelerBoat 架构图

## 系统概览

AccelerBoat 是 OCI 镜像仓库加速器，作为 **Registry 代理** 运行：客户端（Docker/containerd）将 registry 配置指向代理地址，代理拦截 OCI 请求并从本地或集群内缓存提供服务，必要时回源到上游仓库。

---

## 高层架构图（Mermaid）

```mermaid
flowchart TB
    subgraph Clients["客户端"]
        Docker["Docker / containerd"]
    end

    subgraph AccelerBoat["AccelerBoat 集群"]
        subgraph NodeA["节点 A (可能为 Master)"]
            HTTP["HTTP/HTTPS Server"]
            RP["Registry Proxy\n(按 Host 路由)"]
            CA["Custom API\n(仅 Master 处理协调)"]
            OCI["OCI Scanner\n(containerd → Redis)"]
            SFW["Static File Watcher\n(层目录 → Redis)"]
            BT["BitTorrent Handler"]
        end
        subgraph NodeB["节点 B (Replica)"]
            HTTP2["HTTP/HTTPS Server"]
            RP2["Registry Proxy"]
            CA2["Custom API\n(转发到 Master)"]
        end
    end

    subgraph Shared["共享存储与协调"]
        Redis[("Redis\n层索引: digest → node/path/type")]
        K8sEP["K8s Endpoints\n(Master 选举)"]
    end

    subgraph External["外部"]
        OrigReg["原始 Registry\n(e.g. registry-1.docker.io)"]
        HTTPProxy["HTTP 代理\n(统一出口/白名单)"]
    end

    Docker -->|"HTTPS (proxy host)"| HTTP
    Docker -->|"HTTPS"| HTTP2
    HTTP --> RP
    HTTP2 --> RP2
    RP -->|"token/manifest/blob"| CA
    RP2 -->|"转发"| CA
    CA -->|"仅 Master"| CA
    CA --> Redis
    CA -->|"无缓存时"| OrigReg
    OrigReg -.->|"可选"| HTTPProxy
    OCI -->|"周期上报 digest"| Redis
    SFW -->|"文件变更"| Redis
    K8sEP -->|"选举"| CA
    CA --> BT
    RP --> BT
```

---

## 请求流与组件关系图

```mermaid
flowchart LR
    subgraph RequestFlow["请求分类与处理"]
        A["1. Service Token\n→ Master /customapi/service-token"]
        B["2. Head/Get Manifest\n→ Master /customapi/head-manifest\n   /customapi/get-manifest"]
        C["3. Get Blob (Layer)\n→ 本地检查 → Master get-layer-info\n   → TCP 或 BitTorrent 拉取"]
    end

    subgraph Master["Master 职责"]
        M1["Token 缓存与多账号重试"]
        M2["Manifest 缓存"]
        M3["查 Redis 找层位置"]
        M4["小层(<20MB) 自己下载"]
        M5["大层 分发给 Worker 节点下载"]
    end

    subgraph Discovery["层发现"]
        R[("Redis")]
        OCI["OCI Scanner\n(containerd 层)"]
        Static["Static Watcher\n(已完成下载的层)"]
    end

    A --> M1
    B --> M2
    C --> M3
    M3 --> R
    R --> OCI
    R --> Static
    M3 --> M4
    M3 --> M5
```

---

## 单次 Blob (Layer) 下载流程

```mermaid
sequenceDiagram
    participant C as 客户端
    participant P as 当前节点 Proxy
    participant M as Master
    participant R as Redis
    participant N as 有层的节点 / Worker
    participant O as 原始 Registry

    C->>P: GET /v2/.../blobs/sha256:xxx
    P->>P: 本地检查 (transfer/small/OCI 路径)
    alt 本地命中
        P->>C: 直接返回文件
    else 本地未命中
        P->>M: POST /customapi/get-layer-info
        M->>O: HEAD blob (取 content-length)
        M->>R: QueryLayers(digest)
        R-->>M: 候选节点列表
        loop 校验候选节点
            M->>N: GET check-static-layer / check-oci-layer
            N-->>M: 存在则返回 path (+ 可选 torrent)
        end
        alt 有节点已有层
            M-->>P: { located, filePath, torrent? }
        else 无缓存
            alt 小层 (<20MB)
                M->>O: 下载
                M->>M: 存 SmallFilePath
                M-->>P: located=Master
            else 大层
                M->>N: GET /customapi/download-layer (选 Worker)
                N->>O: 下载
                N-->>M: located, filePath, torrent?
                M-->>P: { located, filePath, torrent? }
            end
        end
        alt 有 torrent 且启用 BitTorrent
            P->>P: BitTorrent P2P 下载
        else
            P->>N: GET /customapi/transfer-layer-tcp?file=...
            N-->>P: 流式文件
        end
        P->>P: 保存到本地 (transfer path 等)
        P->>C: 从本地返回 blob
    end
```

---

## 部署与进程内组件

```mermaid
flowchart TB
    subgraph Process["AccelerBoat 进程 (每节点)"]
        Main["main.go\n解析配置、启动 Server"]
        Server["AccelerboatServer"]
        Server --> Gin["Gin Engine\n/customapi/*, /metrics"]
        Server --> Handler["ServeHTTP\n按 Host 分发"]
        Handler --> DomainProxy["DomainProxy\n(ProxyHost → OriginalHost)"]
        Handler --> RegistryMirror["RegistryMirror\n(localhost?ns=...)"]
        Server --> TorrentHandler["TorrentHandler"]
        Server --> OCIScanner["OCI Scanner"]
        Server --> StaticWatcher["Static File Watcher"]
        Server --> Cleaner["Image Cleaner"]
    end

    subgraph Goroutines["常驻 Goroutine"]
        G1["runHTTPServer"]
        G2["runHTTPSServer"]
        G3["runOCITickReporter"]
        G4["runStaticFilesWatcher"]
        G5["runOptionFileWatcher"]
        G6["runDiskUsageUpdater"]
    end

    Server --> G1
    Server --> G2
    Server --> G3
    Server --> G4
    Server --> G5
    Server --> G6
```

---

## 数据与路径

| 存储/路径         | 用途 |
|------------------|------|
| **Redis**        | 层索引：digest → (node, path, type, ts)。OCI 层、静态层均上报至此。 |
| **TransferPath** | 已完成下载的层（TCP 拉取），可被 Static Watcher 发现并写入 Redis。 |
| **DownloadPath** | 临时下载目录，不保证完整。 |
| **SmallFilePath**| 小层（如 <20MB）由 Master 下载存放。 |
| **TorrentPath**  | BitTorrent 缓存与种子存储。 |
| **OCIPath**      | 从 containerd 导出的层缓存。 |

---

## 小结

- **入口**：HTTP/HTTPS 按 Host 区分代理类型（DomainProxy / RegistryMirror），请求进入 Registry Proxy。
- **协调**：仅 **Master** 处理 Custom API 中的 token、manifest、get-layer-info 及下载分发；Master 通过 **K8s Endpoints** 选举（可配 masterIP 或 prefer nodes）。
- **发现**：**Redis** 存层位置；**OCI Scanner** 周期扫描 containerd 上报；**Static File Watcher** 监听层目录变更上报。
- **加速**：层优先从本地或集群内节点取（TCP 或 BitTorrent）；无缓存时由 Master 或指定 Worker 从原始 Registry 拉取，并支持 **HTTP 代理** 统一出口。
- **多账号**：同一 Proxy Host 可配置多组用户名/密码，Master 在取 token 时按序重试并缓存。

以上图与表格可从 `docs/architecture-diagram.md` 查看与编辑；在支持 Mermaid 的 Markdown 预览（如 VS Code、GitHub）中可直接渲染为图。
