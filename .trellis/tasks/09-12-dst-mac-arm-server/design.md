# 技术设计：dst-mac-server（Apple Silicon DST 面板）

> 前置阅读：`research/upstream-code-verification.md`、`research/depotdownloader-integration.md`（下文简称 R1/R2 文档，行号引用均在那两份里）。

## 1. 总体架构

```
┌─ docker run（linux/arm64 容器，Ubuntu 22.04）────────────────────┐
│                                                                  │
│  dst-admin-go 面板 (Go, :8082) ← dist/ 前端（取自上游 release）    │
│      │                                                           │
│      ├─ 游戏安装/更新 ──→ DepotDownloader 3.4.0 (.NET 8, arm64 原生)│
│      │                     -app 343050 -os linux -osarch 64       │
│      ├─ 模组下载 ────────→ DepotDownloader -pubfile <id>           │
│      │                                                           │
│      └─ 游戏启停 ──→ screen + box64 → dontstarve..._x64 (x86_64)  │
│                          │  ↑ amd64 运行库（libc/libstdc++/curl）  │
│                          └─ ~/.steam/sdk64/steamclient.so (x86_64)│
│  卷：/app/data（配置+存档+db）  游戏本体 /app/dst-dedicated-server  │
└──────────────────────────────────────────────────────────────────┘
```

- 仓库形态：fork 上游（保留其 git 历史，`upstream` remote 便于同步），在本仓库发布。
- 分支策略：`upstream/master` → 本仓库 `main`；改动尽量小而集中，降低未来 merge 上游的冲突面。
- GPL-3.0 合规：保留 LICENSE 与原版权声明，README 标注 fork 来源与修改点，同许可证发布。

## 2. Go 源码改动（仅 2 个真实调用点 + 1 个封装）

### 2.1 新增封装 `internal/service/steam/depotdownloader.go`

```go
// 职责：定位 DD 可执行文件、拼参数数组、执行、日志透传
func DownloadApp(dir string, beta bool) error
    // exec: <dd> -app 343050 -os linux -osarch 64 -dir <dir> [-branch updatebeta] -validate
func DownloadPubfile(modID string, dir string) error
    // exec: <dd> -pubfile <modID> -dir <dir>
```

- DD 路径解析：环境变量 `DEPOT_DOWNLOADER` > `<cluster.Steamcmd>/DepotDownloader`（面板字段复用）> 默认 `/opt/DepotDownloader/DepotDownloader`。
- `exec.CommandContext` 参数数组，stdout/stderr 合流后写入面板日志（对齐 shellUtils 行为）；执行期间面板已有超时/重试语义沿用的地方保持不变。
- 平台门禁：仅 `runtime.GOOS != "windows"` 分支使用；Windows 逻辑一行不动。
- 超时：游戏安装给足（无外层超时，沿用上游无超时行为），模组下载沿用现状。

### 2.2 替换游戏安装/更新（`internal/service/update/`）

- `LinuxUpdateCommand()` / `linux_update.go` 的 Shell 执行改为调用 `DownloadApp`：
  - 目标目录 = `cluster.Force_install_dir`（`Beta==1` 时为 `<dir>-beta`，保持上游平行目录语义）
  - beta → `-branch updatebeta`
- `EscapePath`/shell 字符串拼接在该路径上退役（参数数组无需转义）；函数保留（别处仍引用），不加新调用。
- 首装与更新统一走这一个入口；entrypoint 的条件首装（§3.2）与面板内"安装/更新"按钮、autoCheck 自动更新（20min 轮询 appmanifest，逻辑平台无关）全部自然复用。

### 2.3 替换模组下载（`internal/service/mod/mod_service.go` getModInfoConfig）

- steamcmd exec + 输出正则解析 → `DownloadPubfile(modId, dir)`，
  `dir = <Mod_download_path>/steamapps/workshop/content/322330/<modId>`
- 路径契约不变（R1 文档 §1.2）：下载成功后 `<dir>/modinfo.lua` 存在即视为成功；DD 已下载跳过逻辑由现有"目录已存在则跳过"承担。
- `-pubfile` 有 `file_url` 的模组走 DD 内部 web 直下，同样落 `-dir`，行为统一。

### 2.4 不改动清单（明确）

- `linux_process.go`（Bin=2664 box64 启动分支上游已有）
- `dstPath/`（死代码，不动以减小 diff）
- v1 模组 CDN 直下路径、面板 API/路由、数据库、前端 `dist/`（直接取上游 release，后端 API 未动故兼容）

## 3. Docker 镜像设计（`Dockerfile`，多阶段，arm64 单架构）

### 3.1 阶段划分

```dockerfile
# stage 1: builder —— golang:1.24 (BUILDPLATFORM 本机原生跑，交叉编译)
#   CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/dst-admin-go cmd/server/main.go
#   （glebarez/sqlite 纯 Go，无 cgo，交叉编译零障碍）
#   同时 wget 上游 release tarball（ARG UPSTREAM_VERSION=1.6.1）提取 dist/、static/ → /out/
#
# stage 2: runtime —— ubuntu:22.04 (linux/arm64)
#   基础依赖: wget curl screen unzip ca-certificates tzdata procps
#   dotnet-runtime-8.0（Microsoft 源，DD 依赖）
#   dpkg --add-architecture amd64 + amd64 运行库（§3.3 清单）
#   box64: 源码编译 -DARM_DYNAREC=ON -DCMAKE_BUILD_TYPE=RelWithDebInfo
#   DepotDownloader_3.4.0 linux-arm64.zip → /opt/DepotDownloader
#   steamcmd_linux.tar.gz → 解包到 /opt/steamcmd（只为 steamclient.so，不执行任何 i386）
#   COPY --from=builder: dst-admin-go / dist/ / static/ / config.yml / docker-entrypoint.sh / docker_dst_config
#   ENTRYPOINT ./docker-entrypoint.sh   EXPOSE 8082/tcp 10888,10998,10999/udp
```

### 3.2 entrypoint（`scripts/docker-arm64/docker-entrypoint.sh`）

1. `ulimit -Sn 10000`（上游同款，screen 卡顿规避）
2. 播种 `/app/data`：`dst_config`（**默认 `Bin=2664`**、`Steamcmd=/opt/DepotDownloader`）、`password.txt`、backup/mod/`DoNotStarveTogether/Cluster_1` 目录（参考 amd64 entrypoint 逻辑）
3. steamclient.so 就位：`mkdir -p ~/.steam/sdk64 && cp /opt/steamcmd/linux64/steamclient.so ~/.steam/sdk64/`
4. **条件首装**：`<dst_dir>/steamapps/appmanifest_343050.acf` 不存在 → 跑 `DownloadApp` 等价命令（`-validate`）；存在 → 秒级跳过（修复上游半成品每次全量 validate 的问题）。环境变量 `DST_FORCE_UPDATE=1` 可强制更新
5. `chmod +x` bin64 二进制 → `exec ./dst-admin-go`

### 3.3 amd64 运行库（构建期装齐，不放 entrypoint）

- 已知必需：`libc6:amd64`、`libstdc++6:amd64`、`libgcc-s1:amd64`、`libcurl3-gnutls:amd64`（DST 链接 libcurl-gnutls.so.4；jammy 包名实施时以 `ldd` 核对为准）
- 验证方法：容器内 `ldd /app/dst-dedicated-server/bin64/dontstarve_dedicated_server_nullrenderer_x64` 无 "not found"

## 4. CI/CD（GitHub Actions）

- `.github/workflows/release.yml`：
  - 触发：`push: tags v*` + `workflow_dispatch`
  - runner：`ubuntu-24.04-arm`（公共仓库免费 arm64，避免 QEMU 下 box64 编译过慢）
  - 步骤：checkout → docker build（多阶段自带交叉编译）→ tag 命名 `docker.io/<NAMESPACE>/dst-mac-server:<ver>` + `:latest` → push Docker Hub（secrets：`DOCKERHUB_USERNAME`/`DOCKERHUB_TOKEN`）→ GitHub Release 附带镜像说明
  - PR/push 到 main：仅 build 不 push（镜像可构建性守门）
- Docker Hub namespace：以用户账号为准，实施时确认后写入 README/workflow env（占位 `NAMESPACE`）。

## 5. 数据卷与端口（对外契约）

| 项 | 值 |
|----|----|
| 面板 | `8082/tcp` |
| 游戏 | `10888/udp`（默认），`10998/10999/udp`（按集群配置） |
| 持久卷 | `/app/data`（面板配置、sqlite、存档、密码） |
| 可选卷 | `/app/dst-dedicated-server`（游戏本体，持久化避免重下 ~2GB）、`/app/data/mod`（模组缓存） |

- README 提供 `docker-compose.yml` 示例（含 udp 端口与卷）。
- 具体目录名以播种的 `docker_dst_config` 为准（实施时与上游 amd64 版对齐，`persistent_storage_root=/app/data`）。

## 6. 风险与缓解（按优先级）

| # | 风险 | 缓解 |
|---|------|------|
| 1 | box64 跑 DST 性能/稳定性未知（方向已被树莓派社区与上游半成品验证） | 真机验收第一优先项；预期首启慢（DYNAREC 预热），README 写明 |
| 2 | amd64 运行库不全导致 DST 启动失败 | `ldd` 核对清单（§3.3），镜像构建后跑冒烟测试 |
| 3 | steamclient.so 缺失导致连不上 Steam 网络 | entrypoint 强制就位（§3.2.3），开服日志验证 |
| 4 | DD 匿名下载偶发限流/失败 | 沿用上游"失败清缓存重试一次"语义；README 提示重试 |
| 5 | 上游同步冲突 | 改动集中（2 文件 + 1 新文件 + scripts/docker-arm64/），不碰死代码 |
| 6 | box64 源码编译拉长镜像构建 | arm64 runner 原生构建；构建缓存（actions/cache 或 buildx cache） |

## 7. 测试与验证策略

- **本机先行**：开发机（Apple Silicon）直接下载 `DepotDownloader-macos-arm64.zip` 验证 §2 命令映射与模组落盘路径（无需 Docker）
- **Go 单测**：命令构造（beta 分支、路径、参数数组）、模组路径契约拼接
- **镜像冒烟**：本地 `docker buildx build --platform linux/arm64` + `ldd` 核对 + 容器内手动跑 DD 安装
- **真机验收**：PRD 验收标准 1-6（用户 Mac 上执行）
