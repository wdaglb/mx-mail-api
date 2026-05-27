package config

import (
	"os"
	"path/filepath"
	"testing"
)

/**
 * TestLoadFileParsesYAML 校验 YAML 配置会映射为运行时配置。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：YAML 解析、默认值填充或校验行为意外变化时测试失败。
 */
func TestLoadFileParsesYAML(t *testing.T) {
	path := writeTempConfig(t, `
http:
  addr: ":18080"
smtp:
  addr: ":12525"
  hostname: "mx.test"
database:
  dsn: "host=localhost user=test dbname=mx_test sslmode=disable"
auth:
  jwt_secret: "test-secret"
  token_ttl_hours: 12
admin:
  initial_user:
    username: "root"
    password: "password"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}

	if cfg.HTTP.Addr != ":18080" {
		t.Fatalf("expected http addr :18080, got %s", cfg.HTTP.Addr)
	}
	if cfg.SMTP.Addr != ":12525" {
		t.Fatalf("expected smtp addr :12525, got %s", cfg.SMTP.Addr)
	}
	if cfg.SMTP.Hostname != "mx.test" {
		t.Fatalf("expected smtp hostname mx.test, got %s", cfg.SMTP.Hostname)
	}
	if cfg.Database.DSN != "host=localhost user=test dbname=mx_test sslmode=disable" {
		t.Fatalf("unexpected database dsn: %s", cfg.Database.DSN)
	}
	if cfg.Auth.JWTSecret != "test-secret" || cfg.Auth.TokenTTLHours != 12 {
		t.Fatalf("unexpected auth config: %#v", cfg.Auth)
	}
	if cfg.Admin.InitialUser.Username != "root" || cfg.Admin.InitialUser.Password != "password" {
		t.Fatalf("unexpected admin config: %#v", cfg.Admin.InitialUser)
	}
}

/**
 * TestLoadFileAppliesDefaults 校验可选监听字段具有安全的本地默认值。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：本地默认值被误删或意外修改时测试失败。
 */
func TestLoadFileAppliesDefaults(t *testing.T) {
	path := writeTempConfig(t, `
database:
  dsn: "host=localhost user=test dbname=mx_test sslmode=disable"
auth:
  jwt_secret: "test-secret"
admin:
  initial_user:
    username: "root"
    password: "password"
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}

	if cfg.HTTP.Addr != ":8080" {
		t.Fatalf("expected default http addr :8080, got %s", cfg.HTTP.Addr)
	}
	if cfg.SMTP.Addr != ":2525" {
		t.Fatalf("expected default smtp addr :2525, got %s", cfg.SMTP.Addr)
	}
	if cfg.SMTP.Hostname != "mx-mail-api.local" {
		t.Fatalf("expected default smtp hostname mx-mail-api.local, got %s", cfg.SMTP.Hostname)
	}
	if cfg.Auth.TokenTTLHours != 24 {
		t.Fatalf("expected default token ttl 24, got %d", cfg.Auth.TokenTTLHours)
	}
}

/**
 * TestLoadFileRequiresDatabaseDSN 校验缺少持久化存储配置时启动不能静默继续。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：缺少 database.dsn 仍被接受时测试失败。
 */
func TestLoadFileRequiresDatabaseDSN(t *testing.T) {
	path := writeTempConfig(t, `
http:
  addr: ":18080"
`)

	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected missing database.dsn to fail")
	}
}

/**
 * writeTempConfig 写入仅供测试使用的 YAML 文件。
 *
 * 参数：
 * - t：测试辅助对象。
 * - content：YAML 内容。
 * 返回值：临时配置文件的绝对路径。
 * 失败条件：文件无法写入时测试失败。
 */
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	return path
}
