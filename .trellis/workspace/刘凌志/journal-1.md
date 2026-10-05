# Journal - 刘凌志 (Part 1)

> AI development session journal
> Started: 2026-09-12

---



## Session 1: v0.1.0 发布收官：真机验收 8 bug 回修 + 密钥治理 + GitHub/Docker Hub 上线
<!-- trellis-session: v=2 fp=f6071227a0575836 -->

**Date**: 2026-09-23
**Task**: v0.1.0 发布收官：真机验收 8 bug 回修 + 密钥治理 + GitHub/Docker Hub 上线
**Branch**: `main`

### Summary

从真机验收走到正式发布。回修 8 个真机 bug：停服优雅化(存档+大厅注销)、UGC 播种管线、CDN-zip 模组本地化+搜索过滤+警告条、删除重装真实语义+执行位修复、备份恢复补路由+原子换入、tail 库 vendor 补丁防面板自杀、上游硬编码密钥移除并全历史清洁(460 提交重写)。发布基建：前端源码补丁通道、仅 Docker Hub 流水线。最终产出 github.com/Leanchi/dst-mac-server + leanchi/dst-mac-server:v0.1.0，本地拉取验证通过。F4 beta 切换用户决定不测；Caves 原版竞态持续观察中。

### Git Commits

| Hash | Message |
|------|---------|
| `26cd85f` | fix+docs: 停服优雅等待 c_shutdown 存档完成，超时才强杀 |
| `287a922` | feat+docs: 启动前从备份库播种 UGC 缓存，摆脱游戏内不稳自下载 |
| `55a77b1` | fix: UGC 播种登记改为逐段独立判断，details-only 必须补 installed 段 |
| `4386b55` | fix: 面板崩溃根因——tail 库 stat 瞬时错误 os.Exit 自杀，vendor 补丁修复 |
| `b8b5048` | fix: 恢复备份 404——补注册丢失的路由并加固恢复语义 |
| `c65b5ed` | fix: 更新游戏后自动补服务器二进制可执行位 |
| `3a70c5f` | fix: 「删除并更新」落地真实清空重装语义（上游 isDelete 参数一直被忽略） |
| `eb329bc` | feat: 模组搜索默认过滤 CDN-zip 型老模组并附警告标记 |
| `334d03b` | feat: 模组搜索增加 CDN-zip 过滤勾选框（前端源码级补丁机制） |
| `64df12c` | feat: 模组卡片对 CDN-zip 型渲染红色警告条 |
| `a3fe1c5` | security: 移除上游硬编码的 Steam API key，改为部署方自行配置 |
| `039fdb7` | chore: 发布目标改为仅 Docker Hub（移除 GHCR 分发） |

### Status

[OK] **Completed**

## Session 2: 模组下载链路大修 + CDN-zip 连环事故 + 端口清零灾难
<!-- trellis-session: v=2 fp=mod-download-https-fix -->

**Date**: 2026-10-05
**Task**: 10-05-mod-download-https-fix
**Branch**: `main`

### Summary

从"下载新模组报网络问题"开始的一天。诊断出三层叠加根因并修复：Steam API 明文 HTTP 国内被掐（6 处改 HTTPS + 调试 CLI 2 处）、DepotDownloader 省略 -app 时联网反查失败即拒跑（补 workshopAppID）、下载失败静默落库返回假成功（getModInfoConfig/getV1ModInfoConfig/readModInfo 错误传播，buildModConfig 统一入口，清 8 条 mod_config='{}' 脏记录）。部署侧：容器 recreate 注入 STEAM_API_KEY 与代理 env（HTTP(S)_PROXY+NO_PROXY 白名单），compose 模板与 README 沉淀三选一网络文档。tugos 版本源验证游戏更新 756039 为最新（F4 复验）。

晚间连环事故：CDN-zip 型模组（347079953/362175979，与 661253977/1898181913 同型）致 Master 启动期无声死亡→"服务器无应答"；本地化救治又发现 all_clients_require_mod=true 的 local- 模组会挡客户端。用户决策：弃用全部 CDN-zip 模组，防线改为订阅/启用入口明确拒绝（任务 10-05-reject-cdnzip-mods 立项，原 auto-localize 方向否决）。真正的无应答主因是下午 16:06 一次房间设置保存把 server_port 清零（空表单字段=破坏性写入，上游缺陷）+世界设置写空；手修端口 3 次被面板写入覆盖（存在无 HTTP 触发的写入者，未定位），最终从 10-02 备份提取原版配置恢复才稳定。

### Git Commits

| Hash | Message |
|------|---------|
| `374368b` | docs: 归档任务 F4 补记游戏更新复验——756039 为最新 |
| `cc0ebfd` | fix: 模组下载链路三处修复——Steam API 全量 HTTPS、DD 补 -app、下载失败如实报错 |
| `f611323` | docs: 下载网络三选一写入 README 与 compose 模板 |
| `cbdd216` | chore(task): 10-05 任务工件与 spec 契约沉淀 |
| `a8ece54` | docs(spec): break-loop 沉淀——写入者契约/空值陷阱/CDN-zip 客户端必装边界/无应答排查 |
| `4c1ed43` | chore(task): B 任务转向——CDN-zip 防线改为订阅/启用入口拒绝 |

### 遗留

- 无 HTTP 触发的 server.ini 写入者未定位（21:35 实测一次，未复现；spec 已记录排查方法）
- 左键拾取问题未闭环确认（已拔复活按钮 mod 待用户验证）
- 面板房间设置空值校验（端口归零缺陷）待立任务
- 14 个僵尸进程待下次容器 recreate 清理

### Status

[OK] **Completed**
