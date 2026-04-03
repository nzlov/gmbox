package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 保存会话所需的最小用户标识。
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// JWTService 负责签发和校验访问令牌。
type JWTService struct {
	secret []byte
	expire time.Duration
}

// NewJWTService 创建 JWT 服务实例。
func NewJWTService(secret string, expire time.Duration) *JWTService {
	return &JWTService{secret: []byte(secret), expire: expire}
}

// Sign 为已登录用户生成带过期时间的令牌。
func (s *JWTService) Sign(userID uint, username string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expire)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// Parse 解析并验证 Cookie 中的 JWT。
func (s *JWTService) Parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("无效令牌")
	}
	return claims, nil
}
