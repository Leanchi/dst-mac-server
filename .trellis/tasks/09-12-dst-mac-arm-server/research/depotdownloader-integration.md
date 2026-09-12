# DepotDownloader 集成要点（2026-09-13，源码级核实）

> 对象：SteamRE/DepotDownloader tag `DepotDownloader_3.4.0`（2025-05-09 发布，最新 release）。
> 已下载 `Program.cs` / `ContentDownloader.cs` 源码核实。

## 1. 资产与运行时

- `DepotDownloader-linux-arm64.zip`（framework-dependent，**需 .NET 8 runtime**，上游 mac Dockerfile 同时装 dotnet-runtime-8.0 佐证）
- 匿名下载：无需用户名密码（上游半成品镜像实证可用，等同 steamcmd `+login anonymous`）

## 2. 命令映射（steamcmd → DepotDownloader）

| 用途 | steamcmd | DepotDownloader |
|------|----------|-----------------|
| 游戏安装/更新 | `+login anonymous +force_install_dir X +app_update 343050 [-beta updatebeta] validate +quit` | `-app 343050 -os linux -osarch 64 -dir X [-branch updatebeta] -validate` |
| 创意工坊模组 | `+workshop_download_item 322330 <id>` | `-pubfile <id>`（自动解析 UGC id） |

增量更新：DD 按 manifest/depot.config 增量拉 chunk，`-validate` 仅校验差异——但 entrypoint 仍不应每次带 `-validate`（首装除外）。

## 3. 关键行为（源码核实，影响 Go 侧设计）

### 3.1 `-dir` 落盘规则 —— ContentDownloader.cs:55-76
- 指定 `-dir X`：文件**直接写入 X**（多 depot 时 last-depot-wins 去重，不建 appid/version 子目录）
- 不指定：`<cwd>/<depotId>/<depotVersion>/`
- X 下会生成 `.DepotDownloader/`（depot.config、staging）→ 无害但模组目录里会多个隐藏目录，拷贝模组时按 modinfo.lua 定位即可

### 3.2 模组下载路径设计
面板产物契约：`<Mod_download_path>/steamapps/workshop/content/322330/<modId>/modinfo.lua`
→ Go 侧直接 `-dir <Mod_download_path>/steamapps/workshop/content/322330/<modId>`，文件落位即满足契约，mod_service 后续逻辑零改动。
（DD 内部：`-pubfile` → `GetPublishedFileDetails` → 有 `file_url` 走 web 直下，否则走 depot 下载；两条路都落到 `-dir`）

### 3.3 执行方式
- Go 侧用 `exec.CommandContext` **参数数组**调用（不经 shell 字符串拼接 → 路径无需 EscapePath 转义，天然防注入/空格问题）
- stdout/stderr 合流转发到面板日志（对齐现有 `shellUtils.Shell` 行为）
- 注意：DD 匿名下载 beta 分支、`-os/-osarch` 过滤参数每次都会连 Steam API

## 4. steamclient.so 处理（游戏运行时依赖，与 DD 无关但同源解决）

- DST 专用服务器运行时需要 x86_64 的 `steamclient.so`（`~/.steam/sdk64/`）；全仓库无处理逻辑（见 upstream-code-verification.md §3.1）
- 方案：镜像构建时下载 `steamcmd_linux.tar.gz` **仅解包**（tarball 含 `linux64/steamclient.so`，x86_64），entrypoint 拷贝到 `$HOME/.steam/sdk64/steamclient.so`
- **绝不执行** tarball 里的 `linux32/steamcmd`（i386，Rosetta/QEMU 跑不了）——下载职责全归 DD
- 验证点：树莓派/box64 社区同款做法，开服日志确认连上 Steam 网络

## 5. x86_64 运行库（box64 跑 DST 前置）

DST bin64 二进制为 x86_64 ELF，容器（arm64）内需安装 amd64 架构运行库：
- 上游半成品只装了 `libc6:amd64 libstdc++6:amd64` —— **不够**（DST 链接 libcurl-gnutls.so.4 等）
- 实施时在 arm64 容器内 `ldd dontstarve_dedicated_server_nullrenderer_x64` 逐个核对，预计需：`libc6:amd64 libstdc++6:amd64 libgcc-s1:amd64 libcurl3-gnutls:amd64`（jammy 包名实施时验证）
- 构建时装好（`dpkg --add-architecture amd64`），不要放 entrypoint（上游半成品的坏味道：每次启动 apt update）

## 6. 本机可先行验证（开发红利）

DepotDownloader 有 `DepotDownloader-macos-arm64.zip` → 开发机（Apple Silicon）上**无需 Docker 即可验证** DD 的下载命令、模组落盘路径、beta 分支行为，缩短实施反馈环。

## 7. B0 实测结论（2026-09-13，macos-arm64 3.4.0 实跑）

### 7.1 可执行形态
- `DepotDownloader-macos-arm64.zip` 为**自包含单文件**（85MB），无需本机 .NET。Linux 版 zip 上游 Dockerfile 配 dotnet-runtime-8.0 → 按 framework-dependent 处理（镜像内装 runtime）
- 匿名下载 343050（public 分支）实测通过：`-os linux -osarch 64` 生效（拿到 depot 343052 的 key）

### 7.2 模组落盘（实测，覆盖 §3.2 假设）
- DST 创意工坊模组**主流为 file_url（CDN zip）型**：实测 3 个知名 mod（362175979/365119238/661253977）全走 `DownloadWebFile`，DD 落盘 `<dir>/<原文件名>.zip` 且**不解压**（zip 内 modinfo.lua 在根部）
- depot 型（file_url 空才走）直接落文件树
- **Go 侧必须加兜底**：下载完成且 `<dir>/modinfo.lua` 不存在时，解压 `<dir>/*.zip` 到 `<dir>`（覆盖前先看清 zip 根是否直接含 modinfo.lua）
- mod id 不能凭记忆：务必用 `ISteamRemoteStorage/GetPublishedFileDetails` 校验 `consumer_app_id==322330`（实测 381397865 是 CS2 物品，报 Unable to locate manifest ID）
- 匿名下载 pubfile 无需登录，报错为空结果时先核对 appid 归属

### 7.3 steamclient.so（重大简化，覆盖 §4）
- **DST Linux 发行包自带 `steamclient.so`**：depot 1006（Steamworks SDK Redist）直接把 `steamclient.so` 下到**游戏目录根**（实测 /tmp/dst-test/steamclient.so），另有 linux64/libsteamwebrtc.so
- → **镜像无需解包 steamcmd tarball**；entrypoint 在游戏安装后把 `<game_dir>/steamclient.so`（兜底找 linux64/）拷贝到 `~/.steam/sdk64/steamclient.so`

### 7.4 网络注意（开发机环境）
- 实测部分 Steam CDN chunk 报 BadGateway（DD 自动重试其他节点），75s 约 10MB——开发机网络受限环境现象；真机/容器内以实际表现为准，README 需写明首次下载需可访问 Steam CDN
- 本机网络：api.steampowered.com 与 Steam CM 通；store.steampowered.com / steamcommunity.com 不通（不影响 DD）
