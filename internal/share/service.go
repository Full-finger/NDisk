package share

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// parseExpiresIn 解析过期时间字符串，返回过期时间（nil 表示永不过期）
func parseExpiresIn(expiresIn string) (*time.Time, error) {
	switch expiresIn {
	case "1d":
		t := time.Now().Add(24 * time.Hour)
		return &t, nil
	case "7d":
		t := time.Now().Add(7 * 24 * time.Hour)
		return &t, nil
	case "30d":
		t := time.Now().Add(30 * 24 * time.Hour)
		return &t, nil
	case "never", "":
		return nil, nil
	default:
		return nil, fmt.Errorf("无效的有效期: %s", expiresIn)
	}
}

// CreateShare 创建分享
func (s *Service) CreateShare(userID uint, req CreateShareRequest) (*Share, error) {
	// 解析过期时间
	expiresAt, err := parseExpiresIn(req.ExpiresIn)
	if err != nil {
		return nil, err
	}

	// 生成分享 token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	shareToken := hex.EncodeToString(b)

	// 处理密码
	var passwordHash string
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		passwordHash = string(hash)
	}

	share := &Share{
		ShareToken:   shareToken,
		UserID:       userID,
		ItemID:       uint(req.ItemID),
		IsFolder:     req.IsFolder,
		ProjectName:  req.ProjectName,
		PasswordHash: passwordHash,
		ExpiresAt:    expiresAt,
	}

	if err := s.db.Create(share).Error; err != nil {
		return nil, err
	}

	return share, nil
}

// GetShareByToken 根据 token 获取分享信息（检查是否过期）
func (s *Service) GetShareByToken(token string) (*Share, error) {
	var share Share
	if err := s.db.Where("share_token = ?", token).First(&share).Error; err != nil {
		return nil, err
	}

	// 检查是否过期
	if share.ExpiresAt != nil && share.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("分享已过期")
	}

	return &share, nil
}

// HasPassword 检查分享是否需要密码
func (s *Service) HasPassword(share *Share) bool {
	return share.PasswordHash != ""
}

// VerifyPassword 验证分享密码
func (s *Service) VerifyPassword(share *Share, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(share.PasswordHash), []byte(password))
	return err == nil
}

// CleanExpiredShares 清理过期的分享记录
func (s *Service) CleanExpiredShares() {
	s.db.Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).Delete(&Share{})
}
