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
