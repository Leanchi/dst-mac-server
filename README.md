# dst-mac-server

> 在 Apple Silicon Mac（M1/M2/M3/M4）的 Docker 上**一键运行《饥荒联机版》(DST) 专用服务器管理面板**。

本项目是 [carrot-hu23/dst-admin-go](https://github.com/carrot-hu23/dst-admin-go) 的 ARM64 适配版：上游面板依赖 Linux steamcmd（32 位 i386 程序，Apple Silicon 的 Rosetta 无法翻译，导致部署失败），本项目用 [DepotDownloader](https://github.com/SteamRE/DepotDownloader)（ARM64 原生）替代 steamcmd 完成游戏安装/更新与创意工坊模组下载，游戏本体（x86_64）经 [box64](https://github.com/ptitSeb/box64) 转译运行。

遵循上游 GPL-3.0 许可证开源，修改点见[与上游的差异](#与上游的差异)。

## 功能

继承上游 dst-admin-go 全部面板能力：

- 🎮 可视化配置房间与世界参数，多集群管理
- 📦 **面板内一键安装 / 自动更新 DST 服务端**（DepotDownloader，支持 updatebeta 测试分支）
- 🧩 **在线搜索、下载、启用创意工坊模组**（DepotDownloader `-pubfile`）
- 💾 存档备份与快照恢复；玩家白名单/黑名单/管理员管理
- 📜 实时日志查看、游戏控制台、宕机自动恢复
- ⏰ 定时任务（自动更新检测、定时公告等）

## 快速开始

要求：Apple Silicon Mac 上的 Docker（Docker Desktop / OrbStack 均可），磁盘空间 ≥ 10GB。

### Docker Compose（推荐）

```bash
git clone https://github.com/<YOUR>/dst-mac-server.git
cd dst-mac-server
docker compose up -d
```

### docker run

```bash
docker run -d --name dst-mac-server \
  -p 8082:8082 \
  -p 10888:10888/udp -p 10998:10998/udp -p 10999:10999/udp \
  -v ./data:/app/data \
  -v ./dst-server:/app/dst-dedicated-server \
  <DOCKERHUB_USER>/dst-mac-server:latest
```

> 镜像尚未发布前可本地构建：`docker buildx build --platform linux/arm64 -t dst-mac-server:dev .`

首次启动会自动通过 DepotDownloader 下载游戏本体（约 2GB，需要能访问 Steam CDN），之后每次启动秒级就绪。浏览器打开 **http://localhost:8082**（默认账号 `admin` / `123456`），创建集群 → 安装游戏 → 开服。

## 配置

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DST_DIR` | `/app/dst-dedicated-server` | 游戏安装目录（容器内路径固定勿改；如需换位置只调整挂载的宿主机侧路径） |
| `DST_FORCE_UPDATE` | 未设置 | 设为 `1` 时启动即执行一次增量更新 |
| `SKIP_GAME_INSTALL` | 未设置 | 设为 `1` 跳过游戏安装，只启动面板（调试用） |

### 数据卷

| 容器路径 | 内容 | 建议 |
|----------|------|------|
| `/app/data` | 面板配置、账号、数据库、存档、备份、模组缓存 | 必须挂载 |
| `/app/dst-dedicated-server` | 游戏本体（约 2GB） | 强烈建议挂载，避免重建容器重复下载 |

### 端口

| 端口 | 协议 | 用途 |
|------|------|------|
| 8082 | TCP | 面板 Web UI |
| 10888 | UDP | 地上世界（默认集群配置） |
| 10998 / 10999 | UDP | 洞穴 / 附加世界 |

朋友连接时使用你 Mac 的公网 IP 或内网穿透地址，游戏端口以面板集群配置为准。

## 常见问题

**首次开服很慢？** 游戏以 x86_64 运行、box64 转译执行，世界首次生成需要几分钟预热，属正常现象，之后会明显变快。

**游戏下载失败 / 报 BadGateway？** DepotDownloader 从 Steam CDN 下载，个别节点在部分网络环境下不可用会自动重试；多次失败请检查网络对 Steam CDN 的可达性（代理环境可给 Docker 配置代理），或重启容器重试（下载进度按 chunk 保留）。

**如何切换测试分支（updatebeta）？** 在面板集群配置中开启 Beta 选项，游戏会安装到平行的 `-beta` 目录，互不影响。

**模组下载后没有 modinfo？** 面板已内置 CDN zip 型模组的自动解压兜底；如遇极端情况删除该模组目录重新下载即可。

## 与上游的差异

基点：[dst-admin-go main](https://github.com/carrot-hu23/dst-admin-go)（d189b9e）。全部改动：

1. **游戏安装/更新改用 DepotDownloader**：新增 `internal/service/steam` 封装（参数数组执行、日志透传、可执行路径解析），`internal/service/update/linux_update.go` 改调；保留 beta 平行目录与失败重试语义；Windows 分支未动
2. **创意工坊模组下载改用 DepotDownloader `-pubfile`**：产物路径契约不变；新增 CDN zip 型模组自动解压兜底
3. **ARM64 Docker 镜像**：多阶段构建（Go 交叉编译 + 上游 companion 仓库 [dst-manage-web2](https://github.com/carrot-hu23/dst-manage-web2) 构建前端），内置 box64（ARM_DYNAREC）、.NET 8 runtime、DepotDownloader 与 x86_64 运行库
4. **entrypoint 优化**（相对上游 scripts/docker-build-mac 半成品）：按 `appmanifest` 判断跳过重复安装（不再每次启动全量校验）、amd64 运行库构建期装齐、steamclient.so 自动就位（ELF64 校验）
5. 面板默认集群配置 `bin=2664`（box64 启动）、`steamcmd=/opt/DepotDownloader`

## 致谢

- [dst-admin-go](https://github.com/carrot-hu23/dst-admin-go) —— 面板全部功能与前端（GPL-3.0）
- [SteamRE/DepotDownloader](https://github.com/SteamRE/DepotDownloader) —— Steam 内容下载
- [ptitSeb/box64](https://github.com/ptitSeb/box64) —— x86_64 → ARM64 用户态转译

## License

GPL-3.0（继承上游）。本项目为上游的衍生作品，不含任何上游的担保；修改之处已在上文声明。
