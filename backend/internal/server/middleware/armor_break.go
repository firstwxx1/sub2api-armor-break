package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ArmorBreak 破甲中间件：在网关边缘把生成类请求体中的 system 指令改写为
// 管理面选中的人格内容，下游客户端与上游转发链路零感知。
//
// 挂载位置：每条网关链 apiKeyAuth 之后（与 groupModelAllowlist 同段）。
// 行为：
//   - 快速路径：服务未注入、破甲未开启、非改写端点、非 JSON 请求体一律直接放行。
//   - WebSocket/Upgrade 请求跳过（Responses WS 首帧由 handler 链路处理，不破甲）。
//   - 人格加载失败 fail-open：记一条告警，按原始请求体放行，不阻断业务。
//   - 改写：读体（PrereadBody 语义）→ TransformBody → ResetRequestBody 回填，
//     下游 handler 零拷贝重读。
func ArmorBreak(svc *service.ArmorBreakService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || c.Request == nil {
			c.Next()
			return
		}
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			c.Next()
			return
		}
		if c.GetHeader("Upgrade") != "" {
			c.Next()
			return
		}
		shape := service.ArmorBreakShapeForPath(c.Request.URL.Path)
		if shape == service.ArmorBreakShapeNone {
			c.Next()
			return
		}
		state, err := svc.State(c.Request.Context())
		if err != nil {
			// 设置读取失败：fail-open，破甲缺席不构成业务故障。
			logger.LegacyPrintf("middleware.armor_break", "Warning: load armor break state failed: %v", err)
			c.Next()
			return
		}
		if state == nil || !state.Enabled {
			c.Next()
			return
		}
		if state.LoadErr != nil {
			logger.LegacyPrintf("middleware.armor_break", "Warning: armor break persona unavailable, passthrough: %v", state.LoadErr)
			c.Next()
			return
		}
		if state.Content == "" {
			c.Next()
			return
		}
		if ct := c.GetHeader("Content-Type"); ct != "" && !strings.Contains(ct, "application/json") {
			c.Next()
			return
		}

		body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			status := http.StatusBadRequest
			message := "Failed to read request body"
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				status = http.StatusRequestEntityTooLarge
				message = "Request body is too large"
			}
			groupModelAllowlistErrorWriter(c)(c, status, message)
			c.Abort()
			return
		}
		requestmodel.ResetRequestBody(c.Request, service.TransformBody(shape, state.Mode, state.Content, body))
		c.Next()
	}
}
