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
	userID := c.GetUint("user_id")
	username := c.GetString("username")
	accessToken := c.GetString("access_token")

	var parentID *uint
	if pid := c.Query("parent_id"); pid != "" {
		if id, err := strconv.ParseUint(pid, 10, 32); err == nil {
			uid := uint(id)
			parentID = &uid
		}
	}

	files, _ := h.fileService.List(userID, parentID)
	folders, _ := h.fileService.ListFolders(userID, parentID)
	breadcrumb, _ := h.fileService.GetBreadcrumb(userID, parentID)

	c.HTML(http.StatusOK, "files", gin.H{
		"title":       "文件管理",
		"username":    username,
		"files":       files,
		"folders":     folders,
		"parentID":    parentID,
		"breadcrumb":  breadcrumb,
		"accessToken": accessToken,
	})
}

// CookieAuthMiddleware 从 refresh token cookie 验证并生成 access token 注入页面
func (h *Handler) CookieAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		refreshToken, err := c.Cookie("refresh_token")
		if err != nil || refreshToken == "" {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		userID, err := h.authService.ValidateRefreshToken(refreshToken)
		if err != nil {
			c.SetCookie("refresh_token", "", -1, "/", "", false, true)
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		user, err := h.authService.GetUserByID(userID)
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// 生成新的 access token 注入到 context，供模板使用
		accessToken, err := h.authService.GenerateAccessToken(user.ID, user.Username)
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("access_token", accessToken)

		c.Next()
	}
}
