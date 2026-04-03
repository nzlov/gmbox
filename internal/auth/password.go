package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword 使用 bcrypt 保存管理员密码，避免配置明文进入数据库。
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePassword 使用 bcrypt 校验登录口令是否匹配数据库中的哈希值。
func ComparePassword(hash string, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
