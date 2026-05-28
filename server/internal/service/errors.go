package service

import "errors"

var (
	// ErrInvalidCredentials 表示用户名或密码不匹配，API 层会统一映射为 401。
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidUser 表示创建用户时缺少用户名、密码或有效角色。
	ErrInvalidUser = errors.New("invalid user")
	// ErrInvalidRole 表示角色不在系统允许的 admin/user 范围内。
	ErrInvalidRole = errors.New("invalid role")
	// ErrInvalidDomain 表示域名为空、包含空白字符或仍使用已废弃的 "*" 通配写法。
	ErrInvalidDomain = errors.New("invalid domain")
	// ErrDomainVerification 表示域名 TXT 所有权验证失败。
	ErrDomainVerification = errors.New("domain verification failed")
	// ErrForbidden 表示当前用户没有管理或查看目标资源的权限。
	ErrForbidden = errors.New("forbidden")
	// ErrNoUsableDomain 表示用户选择的域名不存在或不在当前用户可用范围内。
	ErrNoUsableDomain = errors.New("no usable domain")
	// ErrTemporaryMailboxNotFound 表示收件地址没有对应的临时邮箱申请记录，SMTP 层会按拒收处理。
	ErrTemporaryMailboxNotFound = errors.New("temporary mailbox not found")
	// ErrTemporaryMailboxExpired 表示临时邮箱已经过期，SMTP 层会按拒收处理。
	ErrTemporaryMailboxExpired = errors.New("temporary mailbox expired")
	// ErrInvalidTemporaryMailboxTTL 表示临时邮箱租赁分钟数为空、越界或不在用户允许范围内。
	ErrInvalidTemporaryMailboxTTL = errors.New("invalid temporary mailbox ttl")
	// ErrInvalidMailboxLocalPart 表示用户指定的邮箱名称为空、过长或包含不支持的字符。
	ErrInvalidMailboxLocalPart = errors.New("invalid mailbox local part")
	// ErrMailboxAlreadyExists 表示用户指定的完整邮箱地址已经被申请。
	ErrMailboxAlreadyExists = errors.New("mailbox already exists")
	// ErrDomainMailboxQuotaExceeded 表示域名累计创建邮箱数量已经达到上限。
	ErrDomainMailboxQuotaExceeded = errors.New("domain mailbox quota exceeded")
	// ErrInvalidOpenAIQoS 表示 OpenAI QoS 配置不在允许范围内。
	ErrInvalidOpenAIQoS = errors.New("invalid openai qos")
	// ErrOpenAIQoSExceeded 表示用户已超过 OpenAI 单用户 RPM 限制。
	ErrOpenAIQoSExceeded = errors.New("openai qos exceeded")
)
