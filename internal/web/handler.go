package web

import (
	"net/http"
	"strconv"

	"github.com/Full-finger/NDisk/internal/auth"
	"github.com/Full-finger/NDisk/internal/file"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	authService      *auth.Service
	fileService      *file.Service
	jwtSecret        string
	defaultWallpaper string
}

func NewHandler(authService *auth.Service, fileService *file.Service, jwtSecret string, defaultWallpaper string) *Handler {
	return &Handler{
		authService:      authService,
		fileService:      fileService,
		jwtSecret:        jwtSecret,
		defaultWallpaper: defaultWallpaper,
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
	nickname := c.GetString("nickname")
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
		"title":          "文件管理",
		"username":       username,
		"nickname":       nickname,
		"files":          files,
		"folders":        folders,
		"parentID":       parentID,
		"breadcrumb":     breadcrumb,
		"accessToken":    accessToken,
		"wallpaper_url":  c.GetString("wallpaper_url"),
		"wallpaper_blur": c.GetInt("wallpaper_blur"),
	})
}

// SharesPage 渲染"我的分享"页面（需要认证）
func (h *Handler) SharesPage(c *gin.Context) {
	username := c.GetString("username")
	nickname := c.GetString("nickname")
	accessToken := c.GetString("access_token")

	c.HTML(http.StatusOK, "shares", gin.H{
		"title":          "我的分享",
		"username":       username,
		"nickname":       nickname,
		"accessToken":    accessToken,
		"wallpaper_url":  c.GetString("wallpaper_url"),
		"wallpaper_blur": c.GetInt("wallpaper_blur"),
	})
}

// NFSPage 渲染 NFS Token 管理页面（需要认证）
func (h *Handler) NFSPage(c *gin.Context) {
	username := c.GetString("username")
	nickname := c.GetString("nickname")
	accessToken := c.GetString("access_token")

	c.HTML(http.StatusOK, "nfs", gin.H{
		"title":          "NFS 挂载管理",
		"username":       username,
		"nickname":       nickname,
		"accessToken":    accessToken,
		"wallpaper_url":  c.GetString("wallpaper_url"),
		"wallpaper_blur": c.GetInt("wallpaper_blur"),
	})
}

// ProfilePage 渲染个人资料页面（需要认证）
func (h *Handler) ProfilePage(c *gin.Context) {
	username := c.GetString("username")
	nickname := c.GetString("nickname")
	accessToken := c.GetString("access_token")

	c.HTML(http.StatusOK, "profile", gin.H{
		"title":          "个人资料",
		"username":       username,
		"nickname":       nickname,
		"accessToken":    accessToken,
		"wallpaper_url":  c.GetString("wallpaper_url"),
		"wallpaper_blur": c.GetInt("wallpaper_blur"),
	})
}

// IndexRedirect 首页重定向：有有效 refresh token 则跳转 /files，否则跳转 /login
func (h *Handler) IndexRedirect(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err == nil && refreshToken != "" {
		if _, err := h.authService.ValidateRefreshToken(refreshToken); err == nil {
			c.Redirect(http.StatusFound, "/files")
			return
		}
	}
	c.Redirect(http.StatusFound, "/login")
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

		nickname := user.Nickname
		if nickname == "" {
			nickname = user.Username
		}
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("nickname", nickname)
		c.Set("access_token", accessToken)

		// 壁纸设置：用户自定义优先，否则用配置默认值
		wallpaperURL := user.WallpaperURL
		if wallpaperURL == "" {
			wallpaperURL = h.defaultWallpaper
		}
		c.Set("wallpaper_url", wallpaperURL)
		c.Set("wallpaper_blur", user.WallpaperBlur)

		c.Next()
	}
}
