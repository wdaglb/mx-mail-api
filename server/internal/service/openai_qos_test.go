package service

import (
	"errors"
	"testing"
	"time"

	"mx-mail-api/internal/storage"
)

/**
 * TestOpenAIQoSLimiterLimitsPerUser 校验 OpenAI QoS 按单用户独立限流。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：同一用户超过 RPM 未拒绝，或不同用户互相影响时测试失败。
 */
func TestOpenAIQoSLimiterLimitsPerUser(t *testing.T) {
	limiter := NewOpenAIQoSLimiter()
	base := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	limiter.SetNowForTest(func() time.Time { return base })

	userA := storage.User{ID: 1, OpenAIQoSRPM: 2}
	userB := storage.User{ID: 2, OpenAIQoSRPM: 1}
	if err := limiter.Allow(userA); err != nil {
		t.Fatalf("expected first userA request allowed, got %v", err)
	}
	if err := limiter.Allow(userA); err != nil {
		t.Fatalf("expected second userA request allowed, got %v", err)
	}
	if err := limiter.Allow(userA); !errors.Is(err, ErrOpenAIQoSExceeded) {
		t.Fatalf("expected userA to be limited, got %v", err)
	}

	if err := limiter.Allow(userB); err != nil {
		t.Fatalf("expected userB first request to be independent, got %v", err)
	}
	if err := limiter.Allow(userB); !errors.Is(err, ErrOpenAIQoSExceeded) {
		t.Fatalf("expected userB to be limited by its own rpm, got %v", err)
	}
}

/**
 * TestOpenAIQoSLimiterResetsWindow 校验一分钟窗口推进后会重置计数。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：窗口过期后仍拒绝请求时测试失败。
 */
func TestOpenAIQoSLimiterResetsWindow(t *testing.T) {
	limiter := NewOpenAIQoSLimiter()
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	limiter.SetNowForTest(func() time.Time { return now })

	user := storage.User{ID: 1, OpenAIQoSRPM: 1}
	if err := limiter.Allow(user); err != nil {
		t.Fatalf("expected first request allowed, got %v", err)
	}
	if err := limiter.Allow(user); !errors.Is(err, ErrOpenAIQoSExceeded) {
		t.Fatalf("expected request within same window to be limited, got %v", err)
	}

	now = now.Add(time.Minute)
	if err := limiter.Allow(user); err != nil {
		t.Fatalf("expected request after window reset allowed, got %v", err)
	}
}
