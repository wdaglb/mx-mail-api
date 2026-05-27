package service

import (
	"sync"
	"time"

	"mx-mail-api/internal/storage"
)

/**
 * OpenAIQoSLimiter 按用户维度执行 OpenAI RPM 限流。
 *
 * 字段：
 * - mu：保护窗口状态，避免并发请求同时修改同一用户计数。
 * - windows：用户 ID 到当前一分钟窗口的映射。
 * - now：可注入的时间函数，测试中用于固定时间。
 */
type OpenAIQoSLimiter struct {
	mu      sync.Mutex
	windows map[uint]openAIQoSWindow
	now     func() time.Time
}

type openAIQoSWindow struct {
	start time.Time
	count int
}

/**
 * NewOpenAIQoSLimiter 创建 OpenAI 单用户限流器。
 *
 * 参数：无。
 * 返回值：使用系统时间的一分钟固定窗口限流器。
 * 失败条件：无。
 */
func NewOpenAIQoSLimiter() *OpenAIQoSLimiter {
	return &OpenAIQoSLimiter{
		windows: make(map[uint]openAIQoSWindow),
		now:     time.Now,
	}
}

/**
 * Allow 判断用户是否还能发起一次 OpenAI 请求。
 *
 * 参数：
 * - user：当前用户模型，使用其中的 OpenAIQoSRPM 作为单用户 RPM 上限。
 * 返回值：允许时返回 nil，超过限制时返回 ErrOpenAIQoSExceeded。
 * 失败条件：无；非法或空配置会通过 OpenAIQoSRPM 兜底为默认值。
 */
func (limiter *OpenAIQoSLimiter) Allow(user storage.User) error {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	rpm := OpenAIQoSRPM(user)
	now := limiter.now()
	current := limiter.windows[user.ID]
	if current.start.IsZero() || now.Sub(current.start) >= time.Minute {
		limiter.windows[user.ID] = openAIQoSWindow{start: now, count: 1}
		return nil
	}
	if current.count >= rpm {
		return ErrOpenAIQoSExceeded
	}

	current.count++
	limiter.windows[user.ID] = current
	return nil
}

/**
 * SetNowForTest 替换时间函数，仅供单元测试控制窗口推进。
 *
 * 参数：
 * - now：测试时间函数；nil 会恢复系统时间。
 * 返回值：无。
 * 失败条件：无。
 */
func (limiter *OpenAIQoSLimiter) SetNowForTest(now func() time.Time) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if now == nil {
		limiter.now = time.Now
		return
	}
	limiter.now = now
}
