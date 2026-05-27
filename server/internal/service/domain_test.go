package service

import (
	"context"
	"errors"
	"net"
	"testing"

	"mx-mail-api/internal/config"
	"mx-mail-api/internal/storage"
)

/**
 * TestVerifyDomainOwnershipAcceptsObservedTXT 校验已解析到公网的 TXT 值能通过所有权验证。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：记录名归一化、稳定验证值计算或 TXT 匹配规则退化时测试失败。
 */
func TestVerifyDomainOwnershipAcceptsObservedTXT(t *testing.T) {
	service := NewDomainService(nil, config.Config{
		SMTP: config.SMTPConfig{
			Addr:     ":2525",
			Hostname: "mail.example.com",
		},
	})
	service.SetTXTLookupForTest(func(_ context.Context, name string) ([]string, error) {
		if name != "a1vayf3yvmfg.meido.cloud" {
			t.Fatalf("unexpected TXT lookup name: %s", name)
		}

		return []string{"6023bb91a9700cb1f484d78f8b885a66"}, nil
	})

	err := service.VerifyDomainOwnership(context.Background(), storage.User{ID: 1, Username: "admin"}, "meido.cloud", DomainVerificationInput{
		Name:  "a1vayf3yvmfg.meido.cloud",
		Value: "6023bb91a9700cb1f484d78f8b885a66",
	})
	if err != nil {
		t.Fatalf("expected verification to pass, got %v", err)
	}
}

/**
 * TestLookupTXTWithFallbackUsesPublicDNS 校验系统 DNS 查询失败后会尝试备用 DNS。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：系统 DNS 失败后直接返回，或备用 DNS 查询结果无法透出时测试失败。
 */
func TestLookupTXTWithFallbackUsesPublicDNS(t *testing.T) {
	records, err := lookupTXTWithResolvers(
		context.Background(),
		"a1vayf3yvmfg.meido.cloud",
		func(context.Context, string) ([]string, error) {
			return nil, errors.New("system resolver unavailable")
		},
		[]txtLookupFunc{
			func(context.Context, string) ([]string, error) {
				return []string{"6023bb91a9700cb1f484d78f8b885a66"}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("expected fallback DNS to pass, got %v", err)
	}
	if len(records) != 1 || records[0] != "6023bb91a9700cb1f484d78f8b885a66" {
		t.Fatalf("unexpected fallback records: %#v", records)
	}
}

/**
 * TestLookupTXTWithFallbackReturnsLastError 校验所有解析器失败时返回最后一个错误。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：错误吞掉或返回空错误时测试失败。
 */
func TestLookupTXTWithFallbackReturnsLastError(t *testing.T) {
	expected := &net.DNSError{Err: "no such host", Name: "missing.example"}
	_, err := lookupTXTWithResolvers(
		context.Background(),
		"missing.example",
		func(context.Context, string) ([]string, error) {
			return nil, errors.New("system resolver unavailable")
		},
		[]txtLookupFunc{
			func(context.Context, string) ([]string, error) {
				return nil, expected
			},
		},
	)
	if !errors.Is(err, expected) {
		t.Fatalf("expected last fallback error, got %v", err)
	}
}
