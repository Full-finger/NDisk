package auth

import (
	"errors"
	"time"

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
