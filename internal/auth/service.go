package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	AccessTokenExpiry  = 15 * time.Minute
	RefreshTokenExpiry = 7 * 24 * time.Hour
)

type Service struct {
	db        *gorm.DB
	jwtSecret string
}

func NewService(db *gorm.DB, jwtSecret string) *Service {
	return &Service{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

// 注册
func (s *Service) Register(req *RegisterRequest) (*User, error) {
	if err := validateUsername(req.Username); err != nil {
		return nil, err
	}
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	var existing User
	if err := s.db.Where("username = ?", req.Username).First(&existing).Error; err == nil {
		return nil, errors.New("username already exists")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &User{Username: req.Username, Password: string(hashed)}
	if err := s.db.Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func validateUsername(username string) error {
	username = strings.TrimSpace(username)
	matched, _ := regexp.MatchString("^[a-zA-Z0-9_]+$", username)
	if !matched {
		return errors.New("用户名只能包含字母、数字和下划线")
	}
	return nil
}

func validatePassword(password string) error {
	var hasUpper, hasLower, hasDigit bool
	for _, r := range password {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasUpper {
		return errors.New("密码必须包含至少一个大写字母")
	}
	if !hasLower {
		return errors.New("密码必须包含至少一个小写字母")
	}
	if !hasDigit {
		return errors.New("密码必须包含至少一个数字")
	}
	return nil
}

// GenerateAccessToken 生成短期 Access Token (JWT)
func (s *Service) GenerateAccessToken(userID uint, username string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(AccessTokenExpiry).Unix(),
	})
	return token.SignedString([]byte(s.jwtSecret))
}

// Claims JWT claims 结构
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// ParseToken 解析并验证 JWT token
func ParseToken(tokenString, jwtSecret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(jwtSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

// generateRandomToken 生成安全的随机 token
func generateRandomToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw := hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(hash[:]), nil
}

// GenerateRefreshToken 生成并存储 Refresh Token
func (s *Service) GenerateRefreshToken(userID uint) (string, error) {
	raw, hashed, err := generateRandomToken()
	if err != nil {
		return "", err
	}

	rt := &RefreshToken{
		UserID:    userID,
		Token:     hashed,
		ExpiresAt: time.Now().Add(RefreshTokenExpiry),
	}
	if err := s.db.Create(rt).Error; err != nil {
		return "", err
	}
	return raw, nil
}

// ValidateRefreshToken 验证 refresh token，返回 userID
func (s *Service) ValidateRefreshToken(rawToken string) (uint, error) {
	hash := sha256.Sum256([]byte(rawToken))
	hashed := hex.EncodeToString(hash[:])

	var rt RefreshToken
	if err := s.db.Where("token = ? AND revoked = ? AND expires_at > ?", hashed, false, time.Now()).First(&rt).Error; err != nil {
		return 0, errors.New("invalid or expired refresh token")
	}
	return rt.UserID, nil
}

// RevokeRefreshToken 吊销指定的 refresh token
func (s *Service) RevokeRefreshToken(rawToken string) {
	hash := sha256.Sum256([]byte(rawToken))
	hashed := hex.EncodeToString(hash[:])
	s.db.Model(&RefreshToken{}).Where("token = ?", hashed).Update("revoked", true)
}

// RevokeAllUserTokens 吊销用户的所有 refresh token
func (s *Service) RevokeAllUserTokens(userID uint) {
	s.db.Model(&RefreshToken{}).Where("user_id = ?", userID).Update("revoked", true)
}

// Login 登录：返回 access token + refresh token
func (s *Service) Login(req *LoginRequest) (*LoginResponse, error) {
	var user User
	if err := s.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid username or password")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, errors.New("invalid username or password")
	}

	accessToken, err := s.GenerateAccessToken(user.ID, user.Username)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.GenerateRefreshToken(user.ID)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: UserInfo{
			ID:       user.ID,
			Username: user.Username,
		},
	}, nil
}

// GetUserByID 根据 ID 获取用户
func (s *Service) GetUserByID(id uint) (*User, error) {
	var user User
	if err := s.db.Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// CleanExpiredRefreshTokens 清理过期的 refresh token
func (s *Service) CleanExpiredRefreshTokens() {
	s.db.Where("expires_at < ? OR revoked = ?", time.Now(), true).Delete(&RefreshToken{})
}
