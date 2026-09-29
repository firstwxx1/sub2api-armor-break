package routes

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterArmorBreakAdminRoutes 注册破甲管理面端点（挂在 /admin 组下，已具备
// adminAuth/auditLog 中间件）。
//
//	GET    /armor-break             破甲状态（开关/模式/当前人格/人格清单）
//	PUT    /armor-break             更新开关/模式/人格（增量，nil 字段不动）
//	GET    /armor-break/personas         人格文件清单
//	GET    /armor-break/personas/:name   读取人格正文
//	PUT    /armor-break/personas/:name   上传/更新人格正文 {"content": "..."}
//	DELETE /armor-break/personas/:name   删除人格文件
func RegisterArmorBreakAdminRoutes(admin *gin.RouterGroup, svc *service.ArmorBreakService) {
	if svc == nil {
		return
	}
	g := admin.Group("/armor-break")

	g.GET("", func(c *gin.Context) {
		state, err := svc.State(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		personas, err := svc.Store().List()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		resp := gin.H{
			"enabled":     state.Enabled,
			"mode":        state.Mode,
			"persona":     state.Persona,
			"personas":    personas,
			"persona_dir": svc.Store().Dir(),
		}
		if state.LoadErr != nil {
			resp["load_error"] = state.LoadErr.Error()
		}
		c.JSON(http.StatusOK, resp)
	})

	g.PUT("", func(c *gin.Context) {
		var upd service.ArmorBreakUpdate
		if err := c.ShouldBindJSON(&upd); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
			return
		}
		state, err := svc.Update(c.Request.Context(), upd)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled": state.Enabled,
			"mode":    state.Mode,
			"persona": state.Persona,
		})
	})

	g.GET("/personas", func(c *gin.Context) {
		personas, err := svc.Store().List()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"personas": personas})
	})

	g.GET("/personas/:name", func(c *gin.Context) {
		name := c.Param("name")
		content, err := svc.Store().Get(name)
		if err != nil {
			armorBreakPersonaError(c, err)
			return
		}
		personas, _ := svc.Store().List()
		size := int64(len(content))
		for _, p := range personas {
			if strings.EqualFold(p.Name, name) {
				size = p.Size
				break
			}
		}
		c.JSON(http.StatusOK, gin.H{"name": name, "size": size, "content": content})
	})

	g.PUT("/personas/:name", func(c *gin.Context) {
		var req struct {
			Content string `json:"content"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
			return
		}
		info, err := svc.Store().Put(c.Param("name"), req.Content)
		if err != nil {
			armorBreakPersonaError(c, err)
			return
		}
		svc.Invalidate()
		c.JSON(http.StatusOK, info)
	})

	g.DELETE("/personas/:name", func(c *gin.Context) {
		name := c.Param("name")
		if err := svc.Store().Delete(name); err != nil {
			armorBreakPersonaError(c, err)
			return
		}
		// 若删掉的正是当前人格，顺手清掉选中项，避免中间件持续 fail-open 告警。
		if state, err := svc.State(c.Request.Context()); err == nil && state != nil && state.Persona == name {
			empty := ""
			_, _ = svc.Update(c.Request.Context(), service.ArmorBreakUpdate{Persona: &empty})
		}
		svc.Invalidate()
		c.JSON(http.StatusOK, gin.H{"deleted": name})
	})
}

// armorBreakPersonaError 把人格仓库错误映射为合适的状态码：
// 「不存在」给 404，其余校验/读写错误给 400/500。
func armorBreakPersonaError(c *gin.Context, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"):
		c.JSON(http.StatusNotFound, gin.H{"error": msg})
	case strings.Contains(msg, "invalid persona name"), strings.Contains(msg, "must end with"),
		strings.Contains(msg, "exceeds"):
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
	}
}
