# PRD：mod 下载失败修复（HTTPS 化 + API key 注入）

## 背景

用户 2026-10-05 在面板下载新模组（3769523601、2753774601）失败，前端提示"网络问题"。

诊断结论（两个根因叠加，任一单独存在都会失败）：

1. **Steam API 明文 HTTP 被掐**：`internal/service/mod/mod_service.go` 共 6 处调用
   `http://api.steampowered.com/IPublishedFileService/{QueryFiles,GetDetails}/v1/`
   （行号 257/366/638/1094/1217/1304）。实测容器内 HTTPS 访问正常（200/0.58s），
   明文 HTTP 无响应。失败发生在 GetDetails 查询模组信息这一步，DepotDownloader
   根本未执行（日志无 "正在执行 DepotDownloader" 记录，PUT 618ms 即返回）。
2. **STEAM_API_KEY 未注入容器**：key 配置在 gitignored 的 docker-compose.override.yml，
   但运行容器创建于 2026-09-24（早于 override 配置），此后仅 restart 未 recreate，
   env 从未注入。`docker inspect` 证实运行容器 0 个 steam 环境变量；
   `/app/config.yml` 的 steamAPIKey 亦为空。key 缺失时 GetDetails 返回 403
   （"获取mod信息失败"），与根因 1 报错形态不同但同样致命。

## 需求

1. `mod_service.go` 内全部 `http://api.steampowered.com` 改为 `https://api.steampowered.com`
   （Go 标准 http.Client 对两者行为一致，无其他代码改动）
2. 重建镜像并 `docker compose up -d` recreate 容器，使 override 中的 STEAM_API_KEY 生效
3. 停服窗口内执行（用户已确认可停服）：优雅停世界（c_shutdown 存档）→ 重建 → 重开
4.（范围追加，2026-10-05）修复"页面显示成功、实际下载失败"的假成功缺陷：
   `getModInfoConfig`/`getV1ModInfoConfig`/`readModInfo` 下载失败、产物缺 modinfo.lua
   时原静默返回空 map，外层照样建 DB 记录并返回成功（DB 实锤：14:52 两条
   mod_config="{}" 的脏记录）。改为错误如实传播（新增 `buildModConfig` 统一四个
   调用点），失败时接口报错、不落脏数据；批量更新循环原有"忽略单模组错误"语义不变
5.（范围追加）下载网络文档化：docker-compose.yml 模板注释块 + README FAQ
   （海外/TUN 零配置、显式代理三行、无代理兜底三条路）；容器代理经
   docker-compose.override.yml 注入（HTTP(S)_PROXY + NO_PROXY 白名单）

## 验收标准

1. `grep -rn "http://api.steampowered" internal/` 零命中；`go build ./...` 与
   `go test ./internal/service/mod/` 通过
2. recreate 后容器 env 含非空 STEAM_API_KEY（只验长度不外泄值）
3. 容器内以注入的 key 走 HTTPS 调 GetDetails（3769523601）返回 200 且含模组标题
4. 端到端：面板/API 重新下载 3769523601 与 2753774601 成功，
   备份库 `/app/data/mod/steamapps/workshop/content/322330/` 出现对应目录
5. 世界重启后面板可正常访问

## 非目标

- 冰箱返鲜等 CDN-zip 模组加载问题（另案）
- Steam API 调用超时/重试机制（沿用现状）
- 347079953/362175979 播种未生效问题（另案观察）
