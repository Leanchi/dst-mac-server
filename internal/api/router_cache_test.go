package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newCacheTestEngine 按生产同样规则（NewRoute → RegisterStaticFile）安装缓存中间件的测试引擎。
// 用 NoRoute 兜底响应，模拟"任意路径都会经过该全局中间件"的生产行为。
func newCacheTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CacheControlMiddleware())
	r.NoRoute(func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r
}

// TestCacheControlMiddleware 覆盖 BUG-1 修复后的三级缓存头分级：
// 动态内容（/api、/ws、/swagger）→ no-store；入口文档 → no-cache；静态资源 → 长缓存 + immutable
func TestCacheControlMiddleware(t *testing.T) {
	const (
		wantNoStore   = "no-store"
		wantNoCache   = "no-cache"
		wantImmutable = "public, max-age=30672000, immutable"
	)

	tests := []struct {
		name string
		path string
		want string
	}{
		// 动态内容：禁止缓存（BUG-1 根因：旧实现把 /api/* JSON 缓存了一年）
		{"API 集群配置", "/api/dst/config", wantNoStore},
		{"API 世界列表", "/api/cluster/level", wantNoStore},
		{"API 静态资源代理", "/api/dst-static/dst/util.js", wantNoStore},
		{"WebSocket", "/ws", wantNoStore},
		{"Swagger 文档", "/swagger/index.html", wantNoStore},
		{"备份恢复（/api 之外的动态接口）", "/backup/restore", wantNoStore},
		// 入口文档：每次向服务器再验证（允许 304）
		{"根路径", "/", wantNoCache},
		{"入口 HTML", "/index.html", wantNoCache},
		{"资源清单", "/asset-manifest.json", wantNoCache},
		// 带 hash 的静态资源：长缓存且不再再验证
		{"assets 脚本", "/assets/index-abc123.js", wantImmutable},
		{"static 样式", "/static/css/main.css", wantImmutable},
		{"misc 资源", "/misc/logo.png", wantImmutable},
		{"站点图标", "/favicon.ico", wantImmutable},
	}

	engine := newCacheTestEngine()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)

			got := rec.Header().Get("Cache-Control")
			if got != tt.want {
				t.Errorf("GET %s Cache-Control = %q, 期望 %q", tt.path, got, tt.want)
			}
		})
	}
}
