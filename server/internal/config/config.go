package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

const DefaultPath = "config.yaml"

/**
 * Config 保存 HTTP 与 SMTP 服务共享的进程级配置。
 *
 * 字段：
 * - HTTP：Gin 服务配置。
 * - SMTP：SMTP/MX TCP 服务配置。
 * - Database：Postgres 连接配置。
 * - Auth：JWT 认证配置。
 * - Admin：首个管理员初始化配置。
 */
type Config struct {
	HTTP     HTTPConfig     `yaml:"http"`
	SMTP     SMTPConfig     `yaml:"smtp"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Admin    AdminConfig    `yaml:"admin"`
}

/**
 * HTTPConfig 描述 Gin HTTP 监听配置。
 *
 * 字段：
 * - Addr：监听地址，例如 ":8080"。
 */
type HTTPConfig struct {
	Addr string `yaml:"addr"`
}

/**
 * SMTPConfig 描述 SMTP/MX 监听配置。
 *
 * 字段：
 * - Addr：TCP 监听地址，例如 ":2525"。
 * - Hostname：用于 greeting 和 EHLO 响应的服务端身份。
 */
type SMTPConfig struct {
	Addr     string `yaml:"addr"`
	Hostname string `yaml:"hostname"`
}

/**
 * DatabaseConfig 描述 GORM 使用的 Postgres 数据库配置。
 *
 * 字段：
 * - DSN：GORM Postgres 驱动接受的 Postgres DSN。
 */
type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

/**
 * AuthConfig 描述 JWT 签名行为。
 *
 * 字段：
 * - JWTSecret：用于签发访问令牌的共享密钥。
 * - TokenTTLHours：令牌有效期，单位为小时。
 */
type AuthConfig struct {
	JWTSecret     string `yaml:"jwt_secret"`
	TokenTTLHours int    `yaml:"token_ttl_hours"`
}

/**
 * AdminConfig 描述首个管理员初始化行为。
 *
 * 字段：
 * - InitialUser：仅当 users 表为空时创建的管理员。
 */
type AdminConfig struct {
	InitialUser InitialAdminUser `yaml:"initial_user"`
}

/**
 * InitialAdminUser 保存初始化管理员的凭据。
 *
 * 字段：
 * - Username：登录用户名。
 * - Password：初始化明文密码，入库前会先哈希。
 */
type InitialAdminUser struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

/**
 * Load 从 CONFIG_PATH 或默认 config.yaml 读取配置。
 *
 * 参数：无。
 * 返回值：已解析并归一化的 Config。
 * 失败条件：配置文件不可读、YAML 无法解析或必填字段缺失时返回错误。
 */
func Load() (Config, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = DefaultPath
	}

	return LoadFile(path)
}

/**
 * LoadFile 从指定 YAML 文件读取配置。
 *
 * 参数：
 * - path：YAML 配置文件路径。
 * 返回值：已解析并归一化的 Config。
 * 失败条件：文件不可读、YAML 无法解析或校验失败时返回错误。
 */
func LoadFile(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return Config{}, err
	}

	applyDefaults(&cfg)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

/**
 * applyDefaults 为可选监听配置填充本地开发默认值。
 *
 * 参数：
 * - cfg：可变配置对象。
 * 返回值：无。
 * 失败条件：无；database.dsn 等必填配置仍由 validate 负责校验。
 */
func applyDefaults(cfg *Config) {
	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = ":8080"
	}
	if cfg.SMTP.Addr == "" {
		cfg.SMTP.Addr = ":2525"
	}
	if cfg.SMTP.Hostname == "" {
		cfg.SMTP.Hostname = "mx-mail-api.local"
	}
	if cfg.Auth.TokenTTLHours == 0 {
		cfg.Auth.TokenTTLHours = 24
	}
}

/**
 * validate 在服务启动前检查必填配置。
 *
 * 参数：
 * - cfg：已归一化的配置对象。
 * 返回值：配置可用时返回 nil。
 * 失败条件：必填值缺失时返回错误。
 */
func validate(cfg Config) error {
	if cfg.Database.DSN == "" {
		return errors.New("database.dsn is required")
	}
	if cfg.Auth.JWTSecret == "" {
		return errors.New("auth.jwt_secret is required")
	}
	if cfg.Admin.InitialUser.Username == "" {
		return errors.New("admin.initial_user.username is required")
	}
	if cfg.Admin.InitialUser.Password == "" {
		return errors.New("admin.initial_user.password is required")
	}

	return nil
}
