package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"mx-mail-api/internal/storage"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

/**
 * Claims 保存 JWT 访问令牌中的认证用户身份。
 *
 * 字段：
 * - UserID：数据库用户 ID。
 * - Username：登录用户名。
 * - Role：授权角色。
 * - RegisteredClaims：JWT 标准过期时间与签发元数据。
 */
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

/**
 * GenerateAPIKey 创建可用于 Bearer 或 X-API-Key 认证的高熵 API Key。
 *
 * 参数：无。
 * 返回值：明文 API Key；调用方只能展示一次，并且只能持久化哈希值。
 * 失败条件：系统安全随机数生成器失败时返回错误。
 */
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return "mxk_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}

/**
 * HashAPIKey 使用 bcrypt 对 API Key 做哈希。
 *
 * 参数：
 * - key：明文 API Key。
 * 返回值：bcrypt 哈希字符串。
 * 失败条件：bcrypt 无法生成哈希时返回错误。
 */
func HashAPIKey(key string) (string, error) {
	return HashPassword(key)
}

/**
 * CheckAPIKey 校验明文 API Key 是否匹配已存储的哈希。
 *
 * 参数：
 * - hash：已存储的 bcrypt 哈希。
 * - key：待校验的明文 API Key。
 * 返回值：匹配时返回 true。
 * 失败条件：无。
 */
func CheckAPIKey(hash string, key string) bool {
	return CheckPassword(hash, key)
}

/**
 * HashPassword 使用 bcrypt 对明文密码做哈希。
 *
 * 参数：
 * - password：来自 API 或初始化配置的明文密码。
 * 返回值：bcrypt 哈希字符串。
 * 失败条件：bcrypt 无法生成哈希时返回错误。
 */
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

/**
 * CheckPassword 校验明文密码是否匹配 bcrypt 哈希。
 *
 * 参数：
 * - hash：已存储的 bcrypt 哈希。
 * - password：待校验的明文密码。
 * 返回值：密码匹配时返回 true。
 * 失败条件：无；格式错误的哈希会被视为不匹配。
 */
func CheckPassword(hash string, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

/**
 * IssueToken 为用户创建已签名的 JWT。
 *
 * 参数：
 * - user：已认证用户。
 * - secret：JWT 签名密钥。
 * - ttl：令牌有效期。
 * 返回值：已签名的访问令牌。
 * 失败条件：密钥为空或签名失败时返回错误。
 */
func IssueToken(user storage.User, secret string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("jwt secret is required")
	}

	now := time.Now()
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

/**
 * ParseToken 校验并解析 JWT 访问令牌。
 *
 * 参数：
 * - tokenText：已签名的 JWT 字符串。
 * - secret：JWT 签名密钥。
 * 返回值：认证后的 Claims。
 * 失败条件：令牌格式错误、已过期、由其他密钥签名或使用非预期签名方法时返回错误。
 */
func ParseToken(tokenText string, secret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected jwt signing method")
		}

		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
