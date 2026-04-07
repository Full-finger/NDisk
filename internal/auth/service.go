package auth

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
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
	// 验证用户名
	if err := validateUsername(req.Username); err != nil {
		return nil, err
	}

	// 验证密码强度
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	// 检查用户名是否存在
	var existing User
	if err := s.db.Where("username = ?", req.Username).First(&existing).Error; err == nil {
		return nil, errors.New("username already exists")
	}

	// 密码加密
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &User{
		Username: req.Username,
		Password: string(hashed),
	}
	if err := s.db.Create(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

// validateUsername 验证用户名
func validateUsername(username string) error {
	username = strings.TrimSpace(username)

	// 检查长度
	if len(username) < 3 {
		return errors.New("用户名至少需要 3 个字符")
	}
	if len(username) > 32 {
		return errors.New("用户名最多 32 个字符")
	}

	// 检查是否只包含字母、数字和下划线
	matched, _ := regexp.MatchString("^[a-zA-Z0-9_]+$", username)
	if !matched {
		return errors.New("用户名只能包含字母、数字和下划线")
	}

	return nil
}

// validatePassword 验证密码强度
func validatePassword(password string) error {
	// 检查长度
	if len(password) < 8 {
		return errors.New("密码至少需要 8 个字符")
	}
	if len(password) > 128 {
		return errors.New("密码最多 128 个字符")
	}

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

	// 检查是否包含大写字母
	if !hasUpper {
		return errors.New("密码必须包含至少一个大写字母")
	}

	// 检查是否包含小写字母
	if !hasLower {
		return errors.New("密码必须包含至少一个小写字母")
	}

	// 检查是否包含数字
	if !hasDigit {
		return errors.New("密码必须包含至少一个数字")
	}

	return nil
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

// 登录
func (s *Service) Login(req *LoginRequest) (*LoginResponse, error) {
	var user User
	if err := s.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid username or password")
		}
		return nil, err
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, errors.New("invalid username or password")
	}

	// 签发 JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	})
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		Token: tokenString,
		User: UserInfo{
			ID:       user.ID,
			Username: user.Username,
		},
	}, nil
}
