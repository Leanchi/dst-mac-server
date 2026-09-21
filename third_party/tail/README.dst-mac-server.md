# dst-mac-server 本地补丁说明

上游：github.com/hpcloud/tail v1.0.0（已停维护），经 go.mod replace 指向本目录。

补丁内容（watch/polling.go 与 watch/inotify.go）：
原实现在 stat 被监听文件遇到非"不存在"错误时调用 util.Fatal → os.Exit(1)，
会杀掉整个面板进程。真机踩坑（2026-09-21）：macOS VirtioFS 在游戏重建
server_chat_log.txt 瞬间返回 EDEADLK（resource deadlock avoided），面板直接
崩溃 → 容器重启策略触发 → 正在启动的游戏世界被硬杀。

补丁语义：stat 失败仅记日志并跳过本轮轮询，等待下一周期重试。
