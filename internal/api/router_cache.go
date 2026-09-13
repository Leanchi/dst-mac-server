package api

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// 缓存头取值（BUG-1 修复）
const (
	// cacheControlNoStore 动态内容（API / WebSocket / Swagger）禁止任何缓存
	cacheControlNoStore = "no-store"
	// cacheControlNoCache 入口文档每次请求都向服务器再验证（允许 304）
	cacheControlNoCache = "no-cache"
	// cacheControlStaticImmutable 文件名带 hash 的静态资源长缓存且不再再验证
	cacheControlStaticImmutable = "public, max-age=30672000, immutable"
)

// CacheControlMiddleware 按路径分级设置 Cache-Control 响应头。
//
// 修复 BUG-1：旧实现对所有响应（含 /api/* JSON）统一设置约一年的长缓存，
// 不同浏览器各自缓存了不同时期的 API 数据，导致世界列表显示不一致。
// 分级规则：
//   - /api、/ws、/swagger、/backup 前缀            → no-store（动态数据不缓存；
//     /backup/restore 是上游注册在 /api 之外的动态 JSON 接口，一并禁止缓存）
//   - /、/index.html、/asset-manifest.json         → no-cache（入口文档每次再验证）
//   - 其余（/assets、/static、/misc、/favicon.ico）→ 长缓存 + immutable（文件名带 hash）
func CacheControlMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Cache-Control", cacheControlForPath(c.Request.URL.Path))
	}
}

// cacheControlForPath 返回请求路径对应的缓存头取值
func cacheControlForPath(path string) string {
	for _, prefix := range []string{"/api", "/ws", "/swagger", "/backup"} {
		if strings.HasPrefix(path, prefix) {
			return cacheControlNoStore
		}
	}
	switch path {
	case "/", "/index.html", "/asset-manifest.json":
		return cacheControlNoCache
	default:
		return cacheControlStaticImmutable
	}
}
