package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// translateValidationErrors 将 gin 的验证错误翻译成中文
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

	// 设置安全的 Cookie
	// 参数: name, value, maxAge, path, domain, secure, httpOnly
	// secure=true 需要 HTTPS，httpOnly=true 防止 XSS 读取 Cookie
	isSecure := c.Request.TLS != nil // 如果是 HTTPS 则启用 secure
	c.SetCookie("token", resp.Token, 86400, "/", "", isSecure, true)

	c.JSON(http.StatusOK, resp)
}

// Logout 登出处理
func (h *Handler) Logout(c *gin.Context) {
	// 清除 Cookie
	c.SetCookie("token", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}
