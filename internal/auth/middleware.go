package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const claimsKey = "auth_claims"

// Middleware 从 HttpOnly Cookie 中恢复登录态，并阻止未认证访问。
func Middleware(cookieName string, jwtSvc *JWTService) gin.HandlerFunc {
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
