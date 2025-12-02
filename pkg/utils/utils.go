package utils

// 一些通用的工具函数
import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// jwt密钥 （先在这里写死，后面换读config）
var JwtSecret = []byte("seckill_secret_key")

// hash密码加密
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// 密码对比
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// 生成JWT token字符串
func GenerateToken(userID uint, username string) (string, error) {
	claims := jwt.MapClaims{
		"uid":      userID,
		"username": username,
		"exp":      time.Now().Add(time.Hour * 72).Unix(), // 过期时间72小时
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(JwtSecret)
}

// 解析JWT token字符串
func ParseToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return JwtSecret, nil
	})
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, err
}
