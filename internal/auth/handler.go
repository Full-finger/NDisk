package auth

import (
	"net/http"

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
		c.SetCookie("refresh_token", "", -1, "/", "", false, true)
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

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"user": UserInfo{
			ID:       user.ID,
			Username: user.Username,
		},
	})
}

// Logout 登出处理
func (h *Handler) Logout(c *gin.Context) {
	if refreshToken, err := c.Cookie("refresh_token"); err == nil && refreshToken != "" {
		h.service.RevokeRefreshToken(refreshToken)
	}
	c.SetCookie("refresh_token", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}
