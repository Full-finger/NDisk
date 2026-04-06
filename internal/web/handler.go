package web

import (
	"net/http"
	"strconv"

	"github.com/Full-finger/NDisk/internal/auth"
	"github.com/Full-finger/NDisk/internal/file"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	authService *auth.Service
	fileService *file.Service
	jwtSecret   string
}

func NewHandler(authService *auth.Service, fileService *file.Service, jwtSecret string) *Handler {
	return &Handler{
		authService: authService,
		fileService: fileService,
		jwtSecret:   jwtSecret,
	}
}

// LoginPage 渲染登录页面
func (h *Handler) LoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login", gin.H{
		"title": "登录",
	})
}

// RegisterPage 渲染注册页面
func (h *Handler) RegisterPage(c *gin.Context) {
	c.HTML(http.StatusOK, "register", gin.H{
		"title": "注册",
	})
}

// FilesPage 渲染文件管理页面（需要认证）
func (h *Handler) FilesPage(c *gin.Context) {
	// 从上下文获取用户信息（由中间件注入）
	userID := c.GetUint("user_id")
	username := c.GetString("username")

	// 获取文件列表
	var parentID *uint
	if pid := c.Query("parent_id"); pid != "" {
		if id, err := strconv.ParseUint(pid, 10, 32); err == nil {
			uid := uint(id)
			parentID = &uid
		}
	}

	files, _ := h.fileService.List(userID, parentID)
	folders, _ := h.fileService.ListFolders(userID, parentID)

	// 获取面包屑导航路径
	breadcrumb, _ := h.fileService.GetBreadcrumb(userID, parentID)

	c.HTML(http.StatusOK, "files", gin.H{
		"title":      "文件管理",
		"username":   username,
		"files":      files,
		"folders":    folders,
		"parentID":   parentID,
		"breadcrumb": breadcrumb,
	})
}

// CookieAuthMiddleware 从 Cookie 验证 JWT 的中间件（用于页面路由）
func (h *Handler) CookieAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Cookie 获取 token
		token, err := c.Cookie("token")
		if err != nil || token == "" {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// 验证 token
		claims, err := auth.ParseToken(token, h.jwtSecret)
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// 设置用户信息到上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)

		c.Next()
	}
}
