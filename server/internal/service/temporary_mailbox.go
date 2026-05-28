package service

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode"

	"mx-mail-api/internal/repository"
	"mx-mail-api/internal/storage"

	"github.com/go-faker/faker/v4"
	"gorm.io/gorm"
)

var permanentMailboxExpiresAt = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

/**
 * TemporaryMailboxService 承载临时邮箱申请、列表和 SMTP 有效性校验。
 *
 * 字段：
 * - mailboxes：临时邮箱仓储。
 * - domains：域名仓储，用于校验用户选择的域名是否可用。
 */
type TemporaryMailboxService struct {
	mailboxes *repository.TemporaryMailboxRepository
	domains   *repository.DomainRepository
}

/**
 * NewTemporaryMailboxService 创建临时邮箱服务。
 *
 * 参数：
 * - mailboxes：临时邮箱仓储。
 * - domains：域名仓储。
 * 返回值：临时邮箱服务实例。
 * 失败条件：无。
 */
func NewTemporaryMailboxService(mailboxes *repository.TemporaryMailboxRepository, domains *repository.DomainRepository) *TemporaryMailboxService {
	return &TemporaryMailboxService{mailboxes: mailboxes, domains: domains}
}

/**
 * Create 为用户申请一个临时邮箱。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * - domain：用户选择的可用域名；为空时从用户可用域名中随机选择。
 * - ttlMinutes：请求体中的租赁分钟数；nil 表示兼容旧客户端，使用用户配置的第一个可选值。
 * - permanent：是否申请永久邮箱；需要用户具备永久邮箱申请权限。
 * 返回值：已创建临时邮箱和本次租赁分钟数。
 * 失败条件：域名不可用、租赁时间不在用户允许范围、邮箱名称生成失败，或数据库插入失败时返回错误。
 */
func (service *TemporaryMailboxService) Create(ctx context.Context, user storage.User, domain string, ttlMinutes *int, permanent bool) (TemporaryMailboxResult, error) {
	normalizedDomain, err := service.resolveDomainForUser(ctx, user.ID, domain)
	if err != nil {
		return TemporaryMailboxResult{}, err
	}

	resolvedTTLMinutes := 0
	expiresAt := permanentMailboxExpiresAt
	if permanent {
		if !CanLeasePermanentMailbox(user) {
			return TemporaryMailboxResult{}, ErrForbidden
		}
	} else {
		resolved, err := ResolveTemporaryMailboxTTLMinutes(user, ttlMinutes)
		if err != nil {
			return TemporaryMailboxResult{}, err
		}
		resolvedTTLMinutes = resolved
		expiresAt = time.Now().Add(time.Duration(resolvedTTLMinutes) * time.Minute)
	}

	for i := 0; i < 5; i++ {
		localPart, err := randomMailboxLocalPart()
		if err != nil {
			return TemporaryMailboxResult{}, err
		}
		mailbox, err := service.mailboxes.Create(ctx, storage.TemporaryMailbox{
			Address:     localPart + "@" + normalizedDomain,
			LocalPart:   localPart,
			Domain:      normalizedDomain,
			OwnerUserID: user.ID,
			ExpiresAt:   expiresAt,
			IsPermanent: permanent,
		})
		if err == nil {
			return TemporaryMailboxResult{
				Mailbox:    mailbox,
				TTLMinutes: resolvedTTLMinutes,
			}, nil
		}
	}

	return TemporaryMailboxResult{}, ErrNoUsableDomain
}

/**
 * ListByOwner 列出用户申请过且仍可用的邮箱。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * 返回值：当前用户未过期的临时邮箱和永久邮箱列表。
 * 失败条件：数据库查询失败时返回错误。
 */
func (service *TemporaryMailboxService) ListByOwner(ctx context.Context, user storage.User) ([]storage.TemporaryMailbox, error) {
	return service.mailboxes.ListByOwner(ctx, user.ID)
}

/**
 * FindOwned 查询当前用户租赁的临时邮箱。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * - address：完整临时邮箱地址。
 * 返回值：匹配且属于当前用户的临时邮箱。
 * 失败条件：邮箱不存在或归属不匹配时返回错误。
 */
func (service *TemporaryMailboxService) FindOwned(ctx context.Context, user storage.User, address string) (storage.TemporaryMailbox, error) {
	mailbox, err := service.mailboxes.FindByAddress(ctx, strings.ToLower(strings.TrimSpace(address)))
	if err != nil {
		return storage.TemporaryMailbox{}, err
	}
	if mailbox.OwnerUserID != user.ID {
		return storage.TemporaryMailbox{}, ErrForbidden
	}

	return mailbox, nil
}

/**
 * EnsureReceivable 校验地址是否为未过期的临时邮箱。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - address：SMTP RCPT TO 中的邮箱地址。
 * 返回值：地址对应未过期临时邮箱时返回 nil。
 * 失败条件：临时邮箱不存在或已过期时返回错误。
 */
func (service *TemporaryMailboxService) EnsureReceivable(ctx context.Context, address string) error {
	mailbox, err := service.mailboxes.FindByAddress(ctx, strings.ToLower(strings.TrimSpace(address)))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrTemporaryMailboxNotFound
	}
	if err != nil {
		return err
	}
	if !mailbox.IsPermanent && time.Now().After(mailbox.ExpiresAt) {
		return ErrTemporaryMailboxExpired
	}

	return nil
}

/**
 * resolveDomainForUser 解析临时邮箱本次申请使用的域名。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - userID：当前用户 ID。
 * - domain：用户提交的域名；为空时随机选择。
 * 返回值：本次申请使用的域名。
 * 失败条件：数据库查询失败时返回错误。
 */
func (service *TemporaryMailboxService) resolveDomainForUser(ctx context.Context, userID uint, domain string) (string, error) {
	normalizedDomain := storage.NormalizeDomain(domain)
	domains, err := service.domains.ListOwnedOrGlobal(ctx, userID)
	if err != nil {
		return "", err
	}

	candidates := make([]string, 0, len(domains))
	for _, item := range domains {
		value := storage.NormalizeDomain(item.Domain)
		if isTemporaryMailboxDomain(value) {
			candidates = append(candidates, value)
		}
		if normalizedDomain != "" && value == normalizedDomain && isTemporaryMailboxDomain(value) {
			return value, nil
		}
	}
	if normalizedDomain != "" {
		return "", ErrNoUsableDomain
	}
	if len(candidates) == 0 {
		return "", ErrNoUsableDomain
	}

	index, err := randomIndex(len(candidates))
	if err != nil {
		return "", err
	}
	return candidates[index], nil
}

/**
 * isTemporaryMailboxDomain 判断域名是否可直接拼接成临时邮箱地址。
 *
 * 参数：
 * - domain：已归一化的域名规则。
 * 返回值：域名不包含已废弃的 "*" 通配符时返回 true。
 * 失败条件：无。
 */
func isTemporaryMailboxDomain(domain string) bool {
	return domain != "" && !strings.Contains(domain, "*")
}

/**
 * randomIndex 使用安全随机数生成列表下标。
 *
 * 参数：
 * - length：候选列表长度。
 * 返回值：0 到 length-1 的随机下标。
 * 失败条件：length 非正数或系统安全随机数生成失败时返回错误。
 */
func randomIndex(length int) (int, error) {
	if length <= 0 {
		return 0, ErrNoUsableDomain
	}

	value, err := rand.Int(rand.Reader, big.NewInt(int64(length)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

/**
 * randomMailboxLocalPart 生成临时邮箱本地部分。
 *
 * 参数：无。
 * 返回值：faker 生成的英文名加 4 位数字后缀，例如 alice4821。
 * 失败条件：系统安全随机数生成失败时返回错误。
 */
func randomMailboxLocalPart() (string, error) {
	suffix, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return "", err
	}

	// faker 生成的姓名更接近真实邮箱命名习惯；仍清洗为小写字母，避免邮箱本地部分出现空格或标点。
	return normalizeMailboxName(faker.FirstName()) + formatMailboxNameSuffix(suffix.Int64()), nil
}

/**
 * formatMailboxNameSuffix 将 0 到 9999 的随机数格式化为固定 4 位。
 *
 * 参数：
 * - value：随机数字。
 * 返回值：补齐前导 0 的 4 位数字字符串。
 * 失败条件：无。
 */
func formatMailboxNameSuffix(value int64) string {
	text := "0000" + strconv.FormatInt(value, 10)
	return text[len(text)-4:]
}

/**
 * normalizeMailboxName 清洗 faker 生成的英文名。
 *
 * 参数：
 * - value：faker 返回的人名。
 * 返回值：仅包含小写英文字母的名称；清洗为空时返回默认名称。
 * 失败条件：无。
 */
func normalizeMailboxName(value string) string {
	var builder strings.Builder
	for _, char := range strings.ToLower(value) {
		if char >= 'a' && char <= 'z' {
			builder.WriteRune(char)
		} else if unicode.IsLetter(char) {
			// 非 ASCII 字母不适合作为对外邮箱名称，避免不同邮件系统处理不一致。
			continue
		}
	}
	if builder.Len() == 0 {
		return "user"
	}

	return builder.String()
}
