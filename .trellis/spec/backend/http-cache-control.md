# HTTP Cache-Control Contract

> Executable contract for `Cache-Control` on every response the gin server emits.
> Born from BUG-1 (2026-09-13): 不同浏览器登录后面板世界列表不一致。

---

## 1. Scope / Trigger

- Trigger: any change touching response headers, static file serving, route registration, or the gin middleware chain in `internal/api/`.
- Why mandatory: browsers **do** cache GET responses (including XHR/fetch JSON) when the server sends `max-age`. A wrong header does not fail loudly — it silently freezes per-browser state for the whole `max-age` window.

## 2. Signatures

```go
// internal/api/router_cache.go
func CacheControlMiddleware() gin.HandlerFunc   // sets Cache-Control per path class
func cacheControlForPath(path string) string    // pure classifier, unit-testable
```

Registered exactly once in `RegisterStaticFile(app)` via `app.Use(CacheControlMiddleware())`.

## 3. Contracts

| Request path | Cache-Control | Rationale |
|---|---|---|
| prefix `/api`, `/ws`, `/swagger`, `/backup` | `no-store` | Dynamic data. Browsers must never store it. |
| exact `/`, `/index.html`, `/asset-manifest.json` | `no-cache` | SPA entry document: may store, must revalidate every load (304 via Last-Modified). |
| everything else (`/assets/*`, `/static/*`, `/misc/*`, `/favicon.ico`) | `public, max-age=30672000, immutable` | Filenames are content-hashed by the Vite build; safe to cache ~1 year. |

Ordering contract (gin semantics): **middleware binds at route-registration time**. `app.Use(...)` only affects routes registered *after* it. That is why `NewRoute` registers `/swagger/*any` after `RegisterStaticFile(app)` (see comment at `internal/api/router.go`). `/hello` is registered before it on purpose (no cache header — byte-identical legacy behavior).

Override contract: handlers may set their own `Cache-Control` later in the chain and win (e.g. SSE handlers in `game_handler.go`, `level_log_handler.go` set `no-cache`). The middleware is the floor, not the ceiling.

## 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| Unauthenticated `/api/*` (401 abort from `Authentication`) | Middleware already ran → `no-store` stands |
| 404 (no route) under `/api/*` | `no-store` (classification is by path, not by handler) |
| 404 outside known prefixes | falls into static bucket — acceptable, no dynamic content |
| Route outside `/api` serving dynamic JSON (e.g. `/backup/restore`) | **must** be added to the no-store prefix list, or moved under `/api` |

## 5. Good / Base / Bad Cases

- Good: `GET /api/cluster/level` → `no-store` — every browser always sees live world list.
- Base: `GET /` → `no-cache` — new image deploys take effect on next normal reload.
- Bad: `GET /api/dst/config` → `public, max-age=30672000` — the BUG-1 case: browser A caches the response from the `Cluster_1` era, browser B caches the `MyDediServer` era, each renders its own frozen world list for a year.

## 6. Tests Required

- `internal/api/router_cache_test.go::TestCacheControlMiddleware` — table-driven, engine installs the middleware exactly as production does. Assertion points:
  - every `/api/*` variant → `no-store` (incl. 404 under `/api`)
  - `/ws`, `/swagger/index.html`, `/backup/restore` → `no-store`
  - `/`, `/index.html`, `/asset-manifest.json` → `no-cache`
  - hashed assets/static/favicon → `public, max-age=30672000, immutable`
- Live smoke after image rebuild: `curl -sI localhost:8082/api/dst/config` → `no-store`; `curl -sI localhost:8082/` → `no-cache`.

## 7. Wrong vs Correct

### Wrong (pre-BUG-1)

```go
app.Use(func(c *gin.Context) {
    // 所有响应统一一年缓存 —— /api/* JSON 也被浏览器冻结
    c.Writer.Header().Set("Cache-Control", "public, max-age=30672000")
})
```

### Correct

```go
app.Use(CacheControlMiddleware()) // 按路径分级：动态 no-store / 入口 no-cache / hash 资源 immutable
```

---

## Convention: 动态路由必须落在 no-store 覆盖面内

**What**: 任何返回 JSON/动态内容的 GET 路由，要么注册在 `/api` 前缀下，要么显式加入 `cacheControlForPath` 的 no-store 前缀表并补测试。

**Why**: 缓存分级按路径前缀划分，游离在外的动态路由会静默落入长缓存桶（BUG-1 同类事故）。`/backup/restore` 就是实例：注册在 `/api` 之外、又是 auth 白名单（`!strings.Contains(path, "/api")` 规则），修复时已补入 no-store 前缀。

**Rule of thumb**: 新路由先问一句"这个响应允许被浏览器存一年吗？"——不允许就必须进 no-store 覆盖面。

## Gotcha: 前端 fetch 也吃 HTTP 缓存

> **Warning**: 浏览器 `fetch`/`axios` 默认 `cache: "default"`，会命中 HTTP 缓存——调试时"API 返回与磁盘文件矛盾"第一反应应是怀疑浏览器缓存。
>
> 验证手法：页面控制台 `fetch(url, {cache: "no-store"})`，或 curl 直接打服务端对比。BUG-1 中改磁盘配置后 API 仍返回旧值，就是普通 fetch 命中缓存根本没发请求。

## Gotcha: 已中毒的浏览器缓存无法由服务端远程清除

`max-age` 未到期前浏览器根本不发请求，服务端改头也不可见。唯一办法是用户一次性硬刷新（Cmd/Ctrl+Shift+R）或清站点数据；发版说明里必须写明。长期方案（前端 axios 统一加 `Cache-Control: no-cache` 请求头强制再验证）需要改动 companion 前端仓库，另行评估。
