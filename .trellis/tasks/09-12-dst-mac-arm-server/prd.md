# PRD: Mac Apple Silicon 部署 DST 服务器方案

## 背景与目标

用户在 Apple Silicon (M 系列) Mac 上用 Docker 部署 dst-admin-go 失败。根因：上游镜像仅 linux/amd64，且 Linux 版 steamcmd 是 32 位 i386 程序（Rosetta 只翻译 x86_64，不支持 i386），游戏下载/更新环节直接失败。

**目标**：做一个可在 Apple Silicon Mac Docker 上正常运行的 DST 服务器管理面板项目，发布到 GitHub + Docker Hub。保留 dst-admin-go 现有两大能力：①游戏自动安装/更新 ②管理端在线搜索/下载创意工坊模组。

**核心待验证问题（用户提出）：用 DepotDownloader 替代 steamcmd 是否可行？**
**结论：可行，且上游已有半成品验证了该方向**（证据见下）。

## 已确认事实（源码/官方文档级证据）

### 根因链
- DST Linux 服务器二进制只有 x86_64，无 ARM 原生版 → Apple Silicon 上必须翻译执行（box64 或 Rosetta）
- Linux steamcmd 主程序是 i386 32 位 → Rosetta/QEMU 均无法可靠运行 → 下载环节挂掉
- 即：下载器必须换（DepotDownloader 有 linux-arm64 原生版），游戏本体用 box64 跑

### dst-admin-go 现状（carrot-hu23/dst-admin-go，GPL-3.0，918 stars，活跃，2026-08 仍在提交）
steamcmd 调用点共 3 处（Linux 路径）：
1. **游戏安装/更新** `internal/service/update/update.go`：构造 `+login anonymous +force_install_dir %s +app_update 343050 [-beta updatebeta] validate +quit`，执行 `<Steamcmd路径>/steamcmd.sh` 或 `steamcmd`（`linux_update.go`，失败清缓存重试一次）。Steamcmd 路径是面板内每集群可配置项（Cluster.SteamCmd）
2. **模组下载** `internal/service/mod/mod_service.go:766-800`：执行 `steamcmd +login anonymous +force_install_dir <mod_download_path> +workshop_download_item 322330 <modId> +quit`，用正则 `Downloaded item \d+ to "([^"]+)"` 解析输出，期望产物在 `<mod_download_path>/steamapps/workshop/content/322330/<modId>/modinfo.lua`；另有 v1 CDN zip 直下路径（file_url，不走 steamcmd）
3. **游戏启动** `internal/service/game/linux_process.go`：按 Cluster.Bin 字段分支，**Bin=2664 时已用 `box64 ./dontstarve_dedicated_server_nullrenderer_x64` 启动（上游原生支持 box64）**

上游半成品：`scripts/docker-build-mac/` 已存在 —— arm64 Ubuntu 22.04 + .NET 8(arm64) + box64(源码编译 ARM_DYNAREC) + DepotDownloader-linux-arm64 + libc6:amd64/libstdc++6:amd64 多架构运行库；entrypoint 每次启动都全量 `-validate`（慢），README 为空、无 CI、无镜像发布。方向与用户设想一致但未完成。

### DepotDownloader 能力（SteamRE 官方 README 原文确认）
- **默认匿名账户**下载 → 完全覆盖 steamcmd `+login anonymous` 场景
- `-app 343050 -os linux -osarch 64 -dir X -validate` ≡ `+force_install_dir X +app_update 343050 validate`（增量下载，只拉变更 chunk）
- `-pubfile <id>` 下载创意工坊物品（自动解析 UGC id）≡ `+workshop_download_item 322330 <id>`
- `-branch updatebeta` ≡ beta 分支
- .NET 8，发布物含 linux-arm64 与 macOS arm64（Homebrew 也有 formula）

### 部署形态
Docker：面板(Web:8082) + 游戏端口 UDP 10888/10998/10999；存档/模组目录 volume 挂载。

## 已确认决策（2026-09-13 用户拍板）

- **D1 技术路线：Fork dst-admin-go 改 Go 源码**（弃 steamcmd shim 方案）
- **D2 镜像范围：仅 linux/arm64**（x86 用户继续用上游镜像，以后有需求再加多架构）
- **D3 命名：GitHub 仓库与 Docker Hub 镜像同名 `dst-mac-server`**

## 需求（已细化）

- R1 在 Apple Silicon Mac Docker 上一条命令启动，面板可创建房间并开服
- R2 面板内安装/自动更新 DST：游戏安装/更新调用点改为 DepotDownloader（含 `-branch updatebeta`，保持 beta 平行目录语义）
- R3 面板内搜索、下载、启用创意工坊模组：`-pubfile` 替代 workshop_download_item，产物路径契约不变
- R4 游戏以 box64 启动（上游 Bin=2664 分支原生支持，镜像默认配置即用）
- R5 项目发布 GitHub（源码，GPL-3.0 合规：保留版权声明、标注修改）+ Docker Hub arm64 镜像，README 完整
- R6 提供本地可复现的多阶段 Dockerfile（非 CI 专属），GitHub Actions 一键构建发布

## 验收标准

1. Apple Silicon Mac 上 `docker run`（或 compose up）后浏览器可访问面板（8082）
2. 面板内完成游戏安装（DepotDownloader 下载 ~2GB）→ 创建世界 → 开服成功 → 客户端可进世界
3. 面板内搜索/下载/启用创意工坊模组 → 进世界模组生效
4. beta 分支切换（正式 ↔ updatebeta）可用
5. 容器二次启动不再全量校验（秒级跳过）
6. `docker push` 后任意 Apple Silicon 主机拉取镜像可用

## 范围外（确认）

- DST 游戏本体 ARM 原生移植（不存在，靠 box64 翻译）
- 非 Docker 的原生 macOS 部署（文档彩蛋可提，不做一等公民）
- 面板功能重写/新 UI；amd64 镜像；给上游提 PR（可选后续，不在本任务）

## 技术风险备忘（design 阶段处理）
- steamclient.so：DST 启动需要 `~/.steam/sdk64/steamclient.so`（x86_64，来自 steamcmd 发行包）；不装 steamcmd 时应在镜像里只解包不执行该 i386 程序。需设计阶段实测
- box64 性能：DST 在 box64/ARM_DYNAREC 下可玩（树莓派社区验证），Apple Silicon 上预期更好；启动慢属正常
- 运行时架构选择：推荐 arm64 容器 + box64（自包含、任何 ARM64 主机可用）；备选 amd64 容器 + Docker Desktop Rosetta（Mac 专属、依赖用户开开关、QEMU 回退极慢）
- 首次下载约 2GB：entrypoint 不应每次启动全量 -validate（上游半成品的问题），需按 appmanifest/目录判断跳过
