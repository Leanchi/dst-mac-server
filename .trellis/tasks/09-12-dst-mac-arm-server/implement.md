# 执行计划：dst-mac-server

> 按 Phase 顺序执行，每个任务有明确验证点。Phase B 与 Phase C 可局部并行，其余串行。

## Phase A：仓库初始化

- [ ] A1 以 tag 1.6.1 对应 commit 为基点 clone 上游到本仓库（保留 git 历史），建 `upstream` remote，`main` 分支为开发基线
      验证：`git log` 含上游历史；`go build ./...` 通过（amd64 本机跑通即可）
- [ ] A2 GPL-3.0 合规处理：保留 LICENSE；README 顶部标注 fork 来源（carrot-hu23/dst-admin-go）与修改说明（占位，Phase E 完善）
      验证：LICENSE 与版权声明完整

## Phase B：Go 源码改造（design.md §2）

- [ ] B0 本机验证 DepotDownloader（开发红利，R2 文档 §6）：下载 `DepotDownloader-macos-arm64.zip`，手动跑
      `-app 343050 -os linux -osarch 64 -dir /tmp/dst-test -validate` 与 `-pubfile <任一模组id> -dir /tmp/mod-test`
      验证：产物落 `/tmp/dst-test`、`/tmp/mod-test/modinfo.lua` 存在 → 证实 `-dir` 直接落盘假设
- [ ] B1 新增 `internal/service/steam/depotdownloader.go`：路径解析（env > cluster.Steamcmd > /opt 默认）+ 参数数组 exec + 日志透传 + 单测
      验证：`go test ./internal/service/steam/` 通过；命令构造单测覆盖 beta/非 beta
- [ ] B2 替换游戏安装/更新：`update/linux_update.go` 改调 `DownloadApp`（Beta==1 → `<dir>-beta` + `-branch updatebeta`）；Windows 分支不动
      验证：单测 + `go build`；`go vet ./...`
- [ ] B3 替换模组下载：`mod_service.go getModInfoConfig` 改调 `DownloadPubfile`，`dir=<Mod_download_path>/steamapps/workshop/content/322330/<modId>`；保留"已存在跳过"；失败语义与现有一致（返回空 map + 日志）
      验证：单测覆盖路径拼接契约
- [ ] B4 交叉编译冒烟：`CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/dst-mac-server cmd/server/main.go`
      验证：产出 arm64 ELF（`file` 核对）

## Phase C：Docker 镜像（design.md §3）

- [ ] C1 编写 `Dockerfile`（多阶段：builder 交叉编译 + 提取上游 dist；runtime 装 dotnet8/box64/DD/steamcmd 解包/amd64 运行库）
      验证：`docker buildx build --platform linux/arm64 -t dst-mac-server:dev .` 本地构建成功
- [ ] C2 编写 `scripts/docker-arm64/docker-entrypoint.sh`（播种 data、steamclient.so、appmanifest 条件首装、DST_FORCE_UPDATE、exec 面板）+ 播种用 `docker_dst_config`（Bin=2664）
      验证：`docker run --rm` 反复启动：首启下载、二启秒过（日志核对）
- [ ] C3 容器内冒烟：`ldd` bin64 无 not found；容器内手动触发面板安装游戏成功
      验证：安装日志为 DD 输出；`appmanifest_343050.acf` 生成
- [ ] C4 `docker-compose.yml` 示例（端口 8082/10888-10999 udp、卷 /app/data 与游戏目录）
      验证：`docker compose up` 可起

## Phase D：CI/CD（design.md §4）

- [ ] D1 `.github/workflows/release.yml`（arm64 runner、tag 触发 build+push、main 分支 build-only 守门）
      验证：push 分支触发 build-only 成功（不 push 镜像）
- [ ] D2 用户配置 Docker Hub secrets（`DOCKERHUB_USERNAME`/`DOCKERHUB_TOKEN`），确认 namespace 写入 workflow 与 README
      验证：（人工）secrets 存在

## Phase E：发布

- [ ] E1 README 重写：定位（Apple Silicon Mac DST 面板）、快速开始（docker run / compose）、与上游差异说明、常见问题（首启慢、beta 切换、模组缓存）、GPL 合规与致谢
      验证：README 自审完整
- [ ] E2 打 tag `v0.1.0` → CI 构建发布 Docker Hub + GitHub Release
      验证：Docker Hub 可拉取 `<NAMESPACE>/dst-mac-server:0.1.0`（arm64）

## Phase F：真机验收（用户 Mac，PRD 验收标准 1-6）

- [ ] F1 一条命令起容器，浏览器访问面板 :8082
- [ ] F2 面板安装游戏 → 创建世界 → 开服 → 客户端进世界（box64 性能记录）
- [ ] F3 搜索/下载/启用创意工坊模组 → 进世界生效
- [ ] F4 正式 ↔ updatebeta 切换可用；二次启动秒级跳过
- [ ] F5 问题记录 → 回修 → （如有通用教训）`trellis-update-spec` 沉淀

## 回滚点

- 每 Phase 一个 commit；Phase B 回滚 = revert Go 改动；Phase C 镜像问题不影响源码；发布回滚 = Docker Hub 删 tag + GitHub 删 release

## 关键命令备忘

```bash
# 交叉编译
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o dst-admin-go cmd/server/main.go
# 本地 arm64 镜像构建
docker buildx build --platform linux/arm64 -t dst-mac-server:dev .
# ldd 核对运行库
docker run --rm dst-mac-server:dev ldd /app/dst-dedicated-server/bin64/dontstarve_dedicated_server_nullrenderer_x64
```
