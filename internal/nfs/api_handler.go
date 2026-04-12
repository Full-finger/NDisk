package nfs

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// APIHandler NFS Token 管理 API
type APIHandler struct {
	db *gorm.DB
}

// NewAPIHandler 创建 NFS API Handler
func NewAPIHandler(db *gorm.DB) *APIHandler {
	return &APIHandler{db: db}
}

// CreateToken 生成 NFS Token
func (h *APIHandler) CreateToken(c *gin.Context) {
	userID := c.GetUint("user_id")

	var req CreateNFSTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	// 生成随机 Token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 Token 失败"})
		return
	}
	token := hex.EncodeToString(b)

	nfsToken := &NFSToken{
		UserID:      userID,
		Token:       token,
		Description: req.Description,
	}

	if err := h.db.Create(nfsToken).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 Token 失败"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":          nfsToken.ID,
		"token":       nfsToken.Token,
		"description": nfsToken.Description,
		"mount_path":  "/token_" + nfsToken.Token,
		"created_at":  nfsToken.CreatedAt,
	})
}

// ListTokens 列出用户的所有 NFS Token
func (h *APIHandler) ListTokens(c *gin.Context) {
	userID := c.GetUint("user_id")

	var tokens []NFSToken
	if err := h.db.Where("user_id = ?", userID).Find(&tokens).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询 Token 失败"})
		return
	}

	type tokenResponse struct {
		ID          uint   `json:"id"`
		Token       string `json:"token"`
		Description string `json:"description"`
		MountPath   string `json:"mount_path"`
		CreatedAt   string `json:"created_at"`
	}

	var resp []tokenResponse
	for _, t := range tokens {
		resp = append(resp, tokenResponse{
			ID:          t.ID,
			Token:       t.Token,
			Description: t.Description,
			MountPath:   "/token_" + t.Token,
			CreatedAt:   t.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, resp)
}

// DeleteToken 删除（吊销）NFS Token
func (h *APIHandler) DeleteToken(c *gin.Context) {
	userID := c.GetUint("user_id")
	tokenID := c.Param("id")

	result := h.db.Where("id = ? AND user_id = ?", tokenID, userID).Delete(&NFSToken{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token 不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Token 已删除"})
}
