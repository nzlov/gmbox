package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gmbox/internal/model"
)

const claimsKey = "auth_claims"

// Middleware 从 HttpOnly Cookie 中恢复登录态，并校验会话版本，确保改密后旧令牌立即失效。
func Middleware(cookieName string, jwtSvc *JWTService, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(cookieName)
		if err != nil || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
			return
		}
		claims, err := jwtSvc.Parse(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "登录已失效"})
			return
		}
		var user model.User
		if err := db.Select("id", "session_version").First(&user, claims.UserID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "登录已失效"})
			return
		}
		if user.SessionVersion != claims.SessionVersion {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "登录已失效，请重新登录"})
			return
		}
		c.Set(claimsKey, claims)
		c.Next()
	}
}

// MustClaims 获取当前请求的登录声明，便于后续接口复用。
func MustClaims(c *gin.Context) *Claims {
	value, _ := c.Get(claimsKey)
	claims, _ := value.(*Claims)
	return claims
}
