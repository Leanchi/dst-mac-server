# Game Process Lifecycle Contract

> Executable contract for starting/stopping DST dedicated server processes (Linux/box64 path).
> Born from BUG-2 (2026-09-20): 停服后 Klei 大厅条目长期残留 + 存档截断风险。

---

## 1. Scope / Trigger

- Trigger: any change to `internal/service/game/linux_process.go` (start/stop/kill/status), or the panel's 世界关闭/重启 flows.
- Why mandatory: `c_shutdown(true)` triggers world-save serialization, which takes **minutes** under box64 translation. An early `kill -9` ① truncates the save mid-serialization, ② kills the process before it can deregister from the Klei lobby, leaving a ghost entry until Klei-side timeout.

## 2. Signatures

```go
// internal/service/game/linux_process.go
const gracefulStopTimeout = 3 * time.Minute // 优雅窗口上限（box64 存档序列化耗时数分钟）
const stopPollInterval   = 3 * time.Second  // 轮询间隔
func waitForProcessExit(status func() (bool, error), timeout, interval time.Duration) bool
```

## 3. Contracts

- Stop order: `shutdownLevel` (send `c_shutdown(true)`) → poll `Status` up to `gracefulStopTimeout` → **only then** `killLevel` (kill -9 as fallback, never the default path).
- Any "stop must return within N seconds" requirement must NOT be met by shrinking the graceful window.
- `Status` is `ps`-based (not screen-based): a Dead screen session does not mean the process is alive. Never use `screen -ls` to judge running state.
- `shutdownLevel`'s background goroutine owns post-exit cleanup of the Dead screen session (`screen -X quit` + `screen -wipe`); box64 does not reap it automatically.
- `waitForProcessExit` treats a status error as "still running" — the deadline then degrades to the kill fallback.

## 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| Process exits before deadline | stop returns nil, log `世界已优雅退出` |
| Still running after deadline | log `优雅关闭超时，使用kill命令强制结束世界`, kill -9 |
| Unit tests | `TestWaitForProcessExit` — 4 branches (immediate exit / exit after N polls / timeout / persistent status error) |
| Live smoke | Panel stop → container log shows `世界已优雅退出`; Klei lobby entry clears within minutes |

## 5. Known Issues

- `windowGameCli.go:445` has a pre-existing `go vet` `atomic.Bool` lock-copy warning (Windows path, untouched).
