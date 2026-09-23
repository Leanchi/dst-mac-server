# 上游 dst-admin-go 代码核实（2026-09-13，shallow clone @ master + release 1.6.1）

> 所有结论均直接读源码验证，非推测。上游：carrot-hu23/dst-admin-go（GPL-3.0，918 stars，go 1.23.0 / toolchain 1.24.7）。

## 1. steamcmd 真实调用点（Linux 路径）

### 1.1 游戏安装/更新 —— 需替换 ✅
- 命令构造：`internal/service/update/update.go:57-71` `LinuxUpdateCommand()`
  - `Bin==86` → `cd <Steamcmd> ; box86 ./linux32/steamcmd <baseCmd>`
  - 否则 → `cd <Steamcmd> ; ./steamcmd.sh <baseCmd>`（存在 steamcmd.sh 时）或 `./steamcmd`
  - baseCmd（`GetBaseUpdateCmd`，update.go:33-46）：`+login anonymous +force_install_dir <dir> +app_update 343050 [-beta updatebeta] validate +quit`
  - **注意**：`Beta==1` 时安装目录加 `-beta` 后缀（`<dir>-beta`），参数加 `-beta updatebeta`——两条平行安装目录并存
  - 路径经 `EscapePath` 转义空格/引号/括号后拼 shell 字符串
- 执行：`internal/service/update/linux_update.go:26-38`，`shellUtils.Shell(updateCommand)`，失败清缓存重试一次
- 更新检测：`linux_update.go:53` 读 `<dir>/steamapps/appmanifest_343050.acf`（平台无关，无需改）

### 1.2 模组下载 —— 需替换 ✅
`internal/service/mod/mod_service.go` `getModInfoConfig()`：
- 已下载检测路径 + 产物查找路径均为：`<Mod_download_path>/steamapps/workshop/content/322330/<modId>/modinfo.lua`（**必须保持的路径契约**，后续拷入集群 mods 目录的逻辑依赖它）
- 未下载时 exec：`steamcmd(或 steamcmd.sh) +login anonymous +force_install_dir <Mod_download_path> +workshop_download_item 322330 <modId> +quit`
- 输出解析：正则 `Downloaded item \d+ to "([^"]+)"`（改用 DepotDownloader 后此正则作废）
- v1 模组走 CDN zip 直下（`getV1ModInfoConfig`），纯 HTTP，平台无关，不动

### 1.3 游戏启动 —— 无需改 ✅
`internal/service/game/linux_process.go:48-90` `launchLevel()`：
- `Bin==2664` → `cd <dir>/bin64 ; screen -d -m -S <session> box64 ./dontstarve_dedicated_server_nullrenderer_x64 -console -cluster <c> -shard <l>`（上游原生支持 box64）
- 其余分支：64/100/86/默认 bin

### 1.4 死代码 —— 不动
`internal/service/dstPath/linux_dst.go` + `dst_path.go` 是 1.1 的复制粘贴，**无任何调用方**。为减少与上游的 diff 保持不动。

## 2. 前端与构建

- 前端源码**不在**仓库内；`internal/api/router.go:65-81` 直接服务预构建 `dist/`（`LoadHTMLGlob("dist/index.html")` + /assets /static 等静态目录）
- 上游 release tarball（`dst-admin-go.1.6.1.tar.gz`，12.8MB，tag `1.6.1`，2026-02-16）内含：`dist/`（预构建前端）、预编译 amd64 二进制、`install_dst_centos.sh`、使用文档 —— **fork 构建时从上游 release 提取 dist 即可，无需维护前端**
- `scripts/build_linux.sh`：仅 `GOOS=linux GOARCH=amd64 go build -o dst-admin-go cmd/server/main.go`
- 数据库：`glebarez/sqlite`（**纯 Go SQLite，无 cgo**）→ `CGO_ENABLED=0 GOARCH=arm64` 交叉编译无障碍
- 入口：`cmd/server/main.go`，端口 8082（config.yml）

## 3. 上游 Docker 两套方案对比

### 3.1 scripts/docker/（amd64 正式版，参考其 entrypoint 逻辑）
- debian:bookworm-slim + i386 架构 + steamcmd 本体
- entrypoint：`ulimit -Sn 10000`；播种 `/app/data`（`dst_config`、`password.txt` admin/123456、backup/mod/`DoNotStarveTogether/Cluster_1` 目录）；steamcmd/游戏缺失时补装；`exec ./dst-admin-go`
- **全仓库无任何 `~/.steam/sdk64/steamclient.so` 处理**（grep 验证）→ ARM 方案必须自行处理（见 depotdownloader-integration.md §3）

### 3.2 scripts/docker-build-mac/（ARM 半成品，方向验证）
- ubuntu:22.04 arm64 + dotnet-runtime-8.0（Microsoft 源）+ box64 源码编译（`-DARM_DYNAREC=ON -DCMAKE_BUILD_TYPE=RelWithDebInfo`）+ DepotDownloader_3.4.0 linux-arm64.zip → /opt/DepotDownloader
- 缺陷（我们要修的）：
  1. entrypoint 每次启动全量 `-validate`（约 2GB 校验，极慢）
  2. entrypoint 运行时才 `dpkg --add-architecture amd64` + 装 `libc6:amd64 libstdc++6:amd64`（且不全——DST 还需 amd64 的 libcurl-gnutls 等运行库，实施时用 `ldd` 核对）
  3. 无 steamclient.so 就位
  4. README 为空、无 CI、无镜像发布
- 该半成品同时证明：DepotDownloader 3.4.0 linux-arm64 匿名下载 343050 可行

## 4. 默认配置（镜像需内置）

- `config.yml`：port 8082，dataDir ./data，autoCheck 各间隔（游戏更新检测 20 分钟——改后走 DD，行为不变）
- `scripts/docker/docker_dst_config` / `docker-build-mac/docker_dst_config`：面板初始集群配置（persistent_storage_root、cluster 名等）；ARM 版需确保默认 `Bin=2664`（box64 启动分支）

## 5. 上游版本基线

- fork 基点：master（1.6.1 release 之后）或直接 tag 1.6.1 —— 实施时定，建议用 release tarball 对应 commit 保证 dist 与后端 API 完全配套

## 6. 「为什么不用 macOS 版游戏本体」（2026-09-13 查证）

- Steam app 343050 确有 macOS depot（343053，"DST Dedicated Server Depot - OSX"；SteamDB 列出 Windows/Linux/macOS 三仓库）
- 但 Docker 容器 = Linux 环境，macOS 程序（Mach-O + macOS 系统库）无法在 Linux 容器内运行 → **只要产品形态是 Docker，游戏本体必然是 Linux 版**
- 原生 macOS 部署（绕开 Docker：DD macos-arm64 下载 OSX depot → Rosetta 2 跑 x86_64 mac 服务器）理论上可行，但面板开服逻辑是 Linux 专属（screen/路径），需新增第三套平台分支 → 已列入 PRD 范围外，README 可作彩蛋提及
- 真机验收前若 OSX depot 仍无社区实践佐证，不纳入任何阶段的备选方案
