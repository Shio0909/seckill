package utils

// 一些通用的工具函数
import (
	"time"

	"seckill/pkg/config"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// getJWTSecret 从配置获取 JWT 密钥
func getJWTSecret() []byte {
	return []byte(config.Get().JWT.Secret)
}

// getJWTExpireTime 从配置获取 JWT 过期时间
func getJWTExpireTime() time.Duration {
	return config.Get().JWT.ExpireTime
}

// HashPassword hash密码加密
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash 密码对比
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateToken 生成JWT token字符串
func GenerateToken(userID uint, username string) (string, error) {
	cfg := config.Get().JWT
	claims := jwt.MapClaims{
		"uid":      userID,
		"username": username,
		"iss":      cfg.Issuer,                               // 签发者
		"exp":      time.Now().Add(cfg.ExpireTime).Unix(),    // 过期时间
		"iat":      time.Now().Unix(),                        // 签发时间
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getJWTSecret())
}

// ParseToken 解析JWT token字符串
func ParseToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return getJWTSecret(), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, err
}
