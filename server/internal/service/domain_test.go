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
		"meido.cloud",
		func(context.Context, string) ([]string, error) {
			return nil, errors.New("system resolver unavailable")
		},
		func(context.Context, string) ([]*net.NS, error) {
			return nil, errors.New("nameserver lookup unavailable")
		},
		func(string) txtLookupFunc {
			return func(context.Context, string) ([]string, error) {
				return nil, errors.New("unexpected discovered dns lookup")
			}
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
 * TestLookupTXTWithResolversUsesDiscoveredDNS 校验 TXT 验证会优先查询用户填写域名的 DNS。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：NS 发现未执行，或用户填写域名的 DNS 返回 TXT 后仍继续走公共 DNS 时测试失败。
 */
func TestLookupTXTWithResolversUsesDiscoveredDNS(t *testing.T) {
	records, err := lookupTXTWithResolvers(
		context.Background(),
		"ltef7of24ugg.edss.bbroot.com",
		"edss.bbroot.com",
		func(context.Context, string) ([]string, error) {
			return nil, errors.New("system resolver unavailable")
		},
		func(_ context.Context, name string) ([]*net.NS, error) {
			if name != "edss.bbroot.com" {
				t.Fatalf("expected NS lookup to use verified subdomain, got %s", name)
			}
			return []*net.NS{{Host: "ns1.dnshe.com."}}, nil
		},
		func(server string) txtLookupFunc {
			if server != "ns1.dnshe.com:53" {
				t.Fatalf("unexpected discovered dns server: %s", server)
			}
			return func(_ context.Context, name string) ([]string, error) {
				if name != "ltef7of24ugg.edss.bbroot.com" {
					t.Fatalf("unexpected TXT lookup name: %s", name)
				}
				return []string{"6023bb91a9700cb1f484d78f8b885a66"}, nil
			}
		},
		nil,
	)
	if err != nil {
		t.Fatalf("expected discovered dns lookup to pass, got %v", err)
	}
	if len(records) != 1 || records[0] != "6023bb91a9700cb1f484d78f8b885a66" {
		t.Fatalf("unexpected discovered dns records: %#v", records)
	}
}

/**
 * TestDiscoverNameServersUsesExactDomain 校验只查待验证域名自身的 DNS。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：查询被放宽到父域，或返回的 DNS 地址没有补齐 53 端口时测试失败。
 */
func TestDiscoverNameServersUsesExactDomain(t *testing.T) {
	queried := make([]string, 0)
	servers, err := discoverNameServers(context.Background(), "edss.bbroot.com", func(_ context.Context, name string) ([]*net.NS, error) {
		queried = append(queried, name)
		if name == "bbroot.com" {
			t.Fatalf("unexpected parent domain NS lookup: %s", name)
		}
		if name == "edss.bbroot.com" {
			return []*net.NS{{Host: "ns1.dnshe.com."}, {Host: "ns3.dnshe.com."}}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: name}
	})
	if err != nil {
		t.Fatalf("expected nameserver discovery to pass, got %v", err)
	}
	if len(servers) != 2 || servers[0] != "ns1.dnshe.com:53" || servers[1] != "ns3.dnshe.com:53" {
		t.Fatalf("unexpected discovered nameservers: %#v", servers)
	}
	want := []string{"edss.bbroot.com"}
	if len(queried) != len(want) {
		t.Fatalf("unexpected ns lookup sequence: %#v", queried)
	}
	for index, value := range want {
		if queried[index] != value {
			t.Fatalf("unexpected ns lookup sequence: %#v", queried)
		}
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
		"example",
		func(context.Context, string) ([]string, error) {
			return nil, errors.New("system resolver unavailable")
		},
		func(context.Context, string) ([]*net.NS, error) {
			return nil, errors.New("nameserver lookup unavailable")
		},
		func(string) txtLookupFunc {
			return func(context.Context, string) ([]string, error) {
				return nil, errors.New("unexpected discovered dns lookup")
			}
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
