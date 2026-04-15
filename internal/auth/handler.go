package auth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

func translateValidationErrors(err error) string {
	errs, ok := err.(validator.ValidationErrors)
	if !ok {
		return err.Error()
	}

	for _, e := range errs {
		switch e.Field() {
		case "Username":
			switch e.Tag() {
			case "required":
				return "用户名不能为空"
			case "min":
				return "用户名至少需要 3 个字符"
			case "max":
				return "用户名最多 32 个字符"
			}
		case "Password":
			switch e.Tag() {
			case "required":
				return "密码不能为空"
			case "min":
				return "密码至少需要 8 个字符"
			case "max":
				return "密码最多 128 个字符"
			}
		}
	}

	return errs[0].Error()
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": translateValidationErrors(err)})
		return
	}

	user, err := h.service.Register(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":       user.ID,
		"username": user.Username,
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": translateValidationErrors(err)})
		return
	}

	resp, err := h.service.Login(&req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// 设置 refresh token 到 HttpOnly Cookie
	isSecure := c.Request.TLS != nil
	c.SetCookie("refresh_token", resp.RefreshToken, 7*24*3600, "/", "", isSecure, true)
	c.SetSameSite(http.SameSiteLaxMode)

	c.JSON(http.StatusOK, resp)
}

// Refresh 用 refresh token cookie 换取新的 access token
func (h *Handler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing refresh token"})
		return
	}

	userID, err := h.service.ValidateRefreshToken(refreshToken)
	if err != nil {
		// 无效 token，清除 cookie
		c.SetCookie("refresh_token", "", -1, "/", "", c.Request.TLS != nil, true)
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// 获取用户信息
	user, err := h.service.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	accessToken, err := h.service.GenerateAccessToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	nickname := user.Nickname
	if nickname == "" {
		nickname = user.Username
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"user": UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Nickname: nickname,
		},
	})
}

// Logout 登出处理
func (h *Handler) Logout(c *gin.Context) {
	if refreshToken, err := c.Cookie("refresh_token"); err == nil && refreshToken != "" {
		h.service.RevokeRefreshToken(refreshToken)
	}
	c.SetCookie("refresh_token", "", -1, "/", "", c.Request.TLS != nil, true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// UpdateProfile 更新用户资料（昵称）
func (h *Handler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "昵称长度需要在 1-64 个字符之间"})
		return
	}

	userID := c.GetUint("user_id")
	if err := h.service.UpdateNickname(userID, req.Nickname); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功", "nickname": req.Nickname})
}

// UpdateWallpaper 更新用户壁纸设置
func (h *Handler) UpdateWallpaper(c *gin.Context) {
	var req UpdateWallpaperRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	// 验证 URL：留空合法（恢复默认），否则必须是 http/https 协议
	if req.WallpaperURL != "" {
		req.WallpaperURL = strings.TrimSpace(req.WallpaperURL)
		parsed, err := url.ParseRequestURI(req.WallpaperURL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "URL 格式无效"})
			return
		}
		scheme := strings.ToLower(parsed.Scheme)
		if scheme != "http" && scheme != "https" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 http/https 协议的 URL"})
			return
		}
		// 拒绝包含控制字符或换行的 URL
		if strings.ContainsAny(req.WallpaperURL, "\r\n\t\x00") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "URL 包含非法字符"})
			return
		}
	}

	userID := c.GetUint("user_id")
	if err := h.service.UpdateWallpaper(userID, req.WallpaperURL, req.WallpaperBlur); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}
