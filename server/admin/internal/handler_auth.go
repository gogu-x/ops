package internal

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/admin/internal/auth"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerAuthRoutes(api *gin.RouterGroup) {
	routes := api.Group("/auth")
	routes.POST("/login", a.login)
	routes.POST("/refresh", a.refresh)
	routes.POST("/logout", a.logout)
}

func (a *AdminService) login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request"})
		return
	}
	result, err := a.auth.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "用户名或密码错误"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) refresh(c *gin.Context) {
	var req model.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request"})
		return
	}
	result, err := a.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "refresh token 无效或已过期"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) logout(c *gin.Context) {
	var req model.RefreshRequest
	_ = c.ShouldBindJSON(&req)
	if err := a.auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "退出登录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminService) me(c *gin.Context) {
	claims, _ := c.Get("claims")
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": claims})
}

func (a *AdminService) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "未登录"})
			c.Abort()
			return
		}
		claims, err := a.auth.ParseAccessToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "登录已过期"})
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}

func requireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := c.Get("claims")
		value, valid := claims.(*auth.Claims)
		if !ok || !valid || value.Role != role {
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "无权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}
