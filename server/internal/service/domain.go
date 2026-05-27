package service

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"

	"mx-mail-api/internal/config"
	"mx-mail-api/internal/repository"
	"mx-mail-api/internal/storage"
)

/**
 * DomainService 承载接受域名的可见性和管理权限规则。
 *
 * 字段：
 * - domains：域名仓储。
 */
type DomainService struct {
	domains    *repository.DomainRepository
	smtpDigest string
	lookupTXT  domainLookupTXT
}

type domainLookupTXT func(ctx context.Context, name string) ([]string, error)
type txtLookupFunc func(ctx context.Context, name string) ([]string, error)

const (
	domainVerificationLabelLength = 12
	domainVerificationAlphabet    = "abcdefghijklmnopqrstuvwxyz0123456789"
)

var fallbackDNSServers = []string{"223.5.5.5:53", "1.1.1.1:53", "8.8.8.8:53"}

/**
 * DomainVerification 表示一次域名 TXT 所有权校验所需的信息。
 *
 * 字段：
 * - Name：需要配置 TXT 的完整记录名。
 * - Value：需要配置的 TXT 值。
 */
type DomainVerification struct {
	Name  string
	Value string
}

/**
 * DomainVerificationInput 表示创建域名时提交的 TXT 校验信息。
 *
 * 字段：
 * - Name：用户实际配置并提交验证的 TXT 记录名。
 * - Value：用户实际配置并提交验证的 TXT 记录值。
 */
type DomainVerificationInput struct {
	Name  string
	Value string
}

/**
 * NewDomainService 创建域名服务。
 *
 * 参数：
 * - domains：域名仓储。
 * - cfg：进程配置，用于把现有 smtp 配置拼接为 TXT 验证摘要。
 * 返回值：域名服务实例。
 * 失败条件：无。
 */
func NewDomainService(domains *repository.DomainRepository, cfg ...config.Config) *DomainService {
	smtpDigest := ""
	if len(cfg) > 0 {
		smtpDigest = smtpConfigDigest(cfg[0].SMTP)
	}

	return &DomainService{domains: domains, smtpDigest: smtpDigest}
}

/**
 * SetTXTLookupForTest 替换 TXT 查询实现，仅供测试注入可控 DNS 响应。
 *
 * 参数：
 * - lookup：测试用 TXT 查询函数。
 * 返回值：无。
 * 失败条件：无；nil 会恢复默认 net.DefaultResolver。
 */
func (service *DomainService) SetTXTLookupForTest(lookup domainLookupTXT) {
	service.lookupTXT = lookup
}

/**
 * ListVisible 返回当前用户可见的域名。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * 返回值：可见域名列表。
 * 失败条件：数据库查询失败时返回错误。
 */
func (service *DomainService) ListVisible(ctx context.Context, user storage.User) ([]storage.AcceptedDomain, error) {
	return service.domains.ListVisible(ctx, user)
}

/**
 * CreateDomain 创建接受域名。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * - domain：用户输入的域名。
 * - requestedOwnerID：管理员可指定的所有者 ID；nil 表示全局域名。
 * 返回值：已创建域名。
 * 失败条件：域名非法或数据库插入失败时返回错误。
 */
func (service *DomainService) CreateDomain(ctx context.Context, user storage.User, domain string, requestedOwnerID *uint) (storage.AcceptedDomain, error) {
	normalized, ok := NormalizeDomainInput(domain)
	if !ok {
		return storage.AcceptedDomain{}, ErrInvalidDomain
	}

	ownerID := &user.ID
	if user.Role == storage.RoleAdmin {
		ownerID = requestedOwnerID
	}

	return service.domains.Create(ctx, storage.AcceptedDomain{Domain: normalized, OwnerUserID: ownerID})
}

/**
 * CreateDomainWithVerification 创建接受域名，并要求 DNS TXT 所有权验证通过。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * - domain：用户输入的域名。
 * - requestedOwnerID：管理员可指定的所有者 ID；nil 表示全局域名。
 * - verification：用户提交的 TXT 记录名和值。
 * 返回值：已创建域名。
 * 失败条件：域名非法、TXT 验证失败或数据库插入失败时返回错误。
 */
func (service *DomainService) CreateDomainWithVerification(ctx context.Context, user storage.User, domain string, requestedOwnerID *uint, verification DomainVerificationInput) (storage.AcceptedDomain, error) {
	normalized, ok := NormalizeDomainInput(domain)
	if !ok {
		return storage.AcceptedDomain{}, ErrInvalidDomain
	}
	if err := service.VerifyDomainOwnership(ctx, user, normalized, verification); err != nil {
		return storage.AcceptedDomain{}, err
	}

	ownerID := &user.ID
	if user.Role == storage.RoleAdmin {
		ownerID = requestedOwnerID
	}

	return service.domains.Create(ctx, storage.AcceptedDomain{Domain: normalized, OwnerUserID: ownerID})
}

/**
 * GenerateVerification 生成指定用户和域名的 TXT 所有权校验信息。
 *
 * 参数：
 * - user：当前用户；验证值按用户 ID、用户名和现有 SMTP 配置稳定生成。
 * - domain：用户准备新增的域名；为空时返回随机记录名前缀，前端可在用户输入域名后补齐完整记录名。
 * 返回值：TXT 记录名和值。
 * 失败条件：域名非法或系统随机数不可用时返回错误。
 */
func (service *DomainService) GenerateVerification(user storage.User, domain string) (DomainVerification, error) {
	label, err := randomAlnumLabel(domainVerificationLabelLength)
	if err != nil {
		return DomainVerification{}, err
	}

	if strings.TrimSpace(domain) == "" {
		return DomainVerification{
			Name:  label,
			Value: service.domainVerificationValue(user),
		}, nil
	}

	normalized, ok := NormalizeDomainInput(domain)
	if !ok {
		return DomainVerification{}, ErrInvalidDomain
	}

	return DomainVerification{
		Name:  label + "." + normalized,
		Value: service.domainVerificationValue(user),
	}, nil
}

/**
 * VerifyDomainOwnership 校验 DNS TXT 记录是否包含当前用户和现有 SMTP 配置的稳定验证值。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户；验证值按用户 ID、用户名和现有 SMTP 配置计算。
 * - domain：已归一化域名。
 * - input：用户提交的 TXT 记录名和值。
 * 返回值：验证通过返回 nil。
 * 失败条件：记录名和值不匹配、DNS 查询失败或 TXT 不存在时返回 ErrDomainVerification。
 */
func (service *DomainService) VerifyDomainOwnership(ctx context.Context, user storage.User, domain string, input DomainVerificationInput) error {
	expectedValue := service.domainVerificationValue(user)
	name, ok := normalizeVerificationRecordName(input.Name, domain)
	value := strings.TrimSpace(input.Value)
	if !ok || value != expectedValue {
		return ErrDomainVerification
	}

	lookup := service.lookupTXT
	if lookup == nil {
		lookup = lookupTXTWithFallback
	}
	records, err := lookup(ctx, name)
	if err != nil {
		return ErrDomainVerification
	}
	for _, record := range records {
		if strings.TrimSpace(record) == value {
			return nil
		}
	}

	return ErrDomainVerification
}

/**
 * lookupTXTWithFallback 查询 TXT 记录，并在系统 DNS 不可用时回退到公共 DNS。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - name：完整 TXT 记录名。
 * 返回值：DNS 返回的 TXT 记录数组。
 * 失败条件：系统 DNS 和备用 DNS 均不可用，或记录不存在时返回最后一次查询错误。
 */
func lookupTXTWithFallback(ctx context.Context, name string) ([]string, error) {
	fallbacks := make([]txtLookupFunc, 0, len(fallbackDNSServers))
	for _, server := range fallbackDNSServers {
		resolver := publicDNSResolver(server)
		fallbacks = append(fallbacks, resolver.LookupTXT)
	}

	return lookupTXTWithResolvers(ctx, name, net.DefaultResolver.LookupTXT, fallbacks)
}

/**
 * lookupTXTWithResolvers 按系统 DNS、备用 DNS 的顺序查询 TXT 记录。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - name：完整 TXT 记录名。
 * - systemLookup：系统默认 TXT 查询函数。
 * - fallbackLookups：备用 DNS 查询函数列表，按优先级顺序执行。
 * 返回值：首个成功解析器返回的 TXT 记录数组。
 * 失败条件：全部解析器均失败时返回最后一次错误。
 */
func lookupTXTWithResolvers(ctx context.Context, name string, systemLookup txtLookupFunc, fallbackLookups []txtLookupFunc) ([]string, error) {
	records, err := systemLookup(ctx, name)
	if len(records) > 0 || err == nil {
		return records, err
	}

	lastErr := err
	for _, lookup := range fallbackLookups {
		records, err = lookup(ctx, name)
		if len(records) > 0 || err == nil {
			return records, err
		}
		lastErr = err
	}

	return nil, lastErr
}

/**
 * publicDNSResolver 创建指向指定 DNS 服务器的 Go Resolver。
 *
 * 参数：
 * - server：DNS 服务器地址，例如 "1.1.1.1:53"。
 * 返回值：只用于域名所有权验证兜底查询的 Resolver。
 * 失败条件：无；网络错误会在实际 LookupTXT 时返回。
 */
func publicDNSResolver(server string) *net.Resolver {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network string, _ string) (net.Conn, error) {
			// 默认优先使用系统 DNS；只有系统解析器失败时才进入这里，避免本机 127.0.0.53 异常影响公网 TXT 验证。
			return dialer.DialContext(ctx, network, server)
		},
	}
}

/**
 * normalizeVerificationRecordName 归一化 TXT 验证记录名。
 *
 * 参数：
 * - name：用户提交的 TXT 记录名，可为完整记录名或仅随机前缀。
 * - domain：已归一化的待验证根域名。
 * 返回值：可查询的完整 TXT 记录名，以及记录名是否符合当前域名验证规则。
 * 失败条件：无；非法记录名通过第二返回值 false 表达。
 */
func normalizeVerificationRecordName(name string, domain string) (string, bool) {
	normalized := strings.TrimSuffix(storage.NormalizeDomain(name), ".")
	normalizedDomain := storage.NormalizeDomain(domain)
	if normalized == "" || normalizedDomain == "" {
		return "", false
	}

	if isAlnumLabel(normalized) {
		return normalized + "." + normalizedDomain, true
	}

	suffix := "." + normalizedDomain
	if !strings.HasSuffix(normalized, suffix) {
		return "", false
	}

	// TXT 记录名使用随机字母数字前缀，必须落在被验证域名之下，避免用外部域名冒充所有权。
	label := strings.TrimSuffix(normalized, suffix)
	if !isAlnumLabel(label) {
		return "", false
	}

	return label + suffix, true
}

/**
 * UpdateDomain 更新当前用户可管理的接受域名。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * - id：域名 ID。
 * - domain：可选新域名。
 * - requestedOwnerID：管理员可指定的所有者 ID；nil 表示全局域名。
 * 返回值：已更新域名。
 * 失败条件：域名不存在、权限不足、域名非法或保存失败时返回错误。
 */
func (service *DomainService) UpdateDomain(ctx context.Context, user storage.User, id uint, domain string, requestedOwnerID *uint) (storage.AcceptedDomain, error) {
	item, err := service.domains.FindByID(ctx, id)
	if err != nil {
		return storage.AcceptedDomain{}, err
	}
	if !CanManageDomain(user, item) {
		return storage.AcceptedDomain{}, ErrForbidden
	}

	if domain != "" {
		normalized, ok := NormalizeDomainInput(domain)
		if !ok {
			return storage.AcceptedDomain{}, ErrInvalidDomain
		}
		item.Domain = normalized
	}
	if user.Role == storage.RoleAdmin {
		item.OwnerUserID = requestedOwnerID
	}

	return service.domains.Save(ctx, item)
}

/**
 * DeleteDomain 删除当前用户可管理的接受域名。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * - id：域名 ID。
 * 返回值：无。
 * 失败条件：域名不存在、权限不足或删除失败时返回错误。
 */
func (service *DomainService) DeleteDomain(ctx context.Context, user storage.User, id uint) error {
	item, err := service.domains.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !CanManageDomain(user, item) {
		return ErrForbidden
	}

	return service.domains.Delete(ctx, item)
}

/**
 * NormalizeDomainInput 校验并归一化接受域名输入。
 *
 * 参数：
 * - domain：用户输入的根域名；不再支持 "*" 通配写法。
 * 返回值：域名有效时返回归一化域名和 true。
 * 失败条件：无。
 */
func NormalizeDomainInput(domain string) (string, bool) {
	normalized := storage.NormalizeDomain(domain)
	return normalized, storage.IsValidDomainPattern(normalized)
}

/**
 * CanManageDomain 检查用户是否可以管理域名。
 *
 * 参数：
 * - user：当前用户。
 * - domain：接受域名模型。
 * 返回值：管理员或普通用户管理自己拥有的非全局域名时返回 true。
 * 失败条件：无。
 */
func CanManageDomain(user storage.User, domain storage.AcceptedDomain) bool {
	if user.Role == storage.RoleAdmin {
		return true
	}
	if domain.OwnerUserID == nil {
		return false
	}

	return *domain.OwnerUserID == user.ID
}

/**
 * domainVerificationValue 按用户 ID、用户名和现有 SMTP 配置生成稳定 TXT 验证值。
 *
 * 参数：
 * - user：当前用户。
 * 返回值：32 位 MD5 验证值。
 * 失败条件：无。
 */
func (service *DomainService) domainVerificationValue(user storage.User) string {
	sum := md5.Sum([]byte(strconv.FormatUint(uint64(user.ID), 10) + user.Username + service.smtpDigest))
	return hex.EncodeToString(sum[:])
}

/**
 * randomAlnumLabel 生成 DNS TXT 验证使用的随机字母数字前缀。
 *
 * 参数：
 * - length：前缀长度。
 * 返回值：仅包含小写字母和数字的随机字符串。
 * 失败条件：系统安全随机数不可用时返回错误。
 */
func randomAlnumLabel(length int) (string, error) {
	var builder strings.Builder
	builder.Grow(length)
	max := big.NewInt(int64(len(domainVerificationAlphabet)))

	for i := 0; i < length; i++ {
		index, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		builder.WriteByte(domainVerificationAlphabet[index.Int64()])
	}

	return builder.String(), nil
}

/**
 * isAlnumLabel 检查 TXT 验证记录名前缀是否仅包含字母和数字。
 *
 * 参数：
 * - value：待检查的记录名前缀。
 * 返回值：全部字符为小写字母或数字时返回 true。
 * 失败条件：无。
 */
func isAlnumLabel(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			continue
		}
		return false
	}

	return true
}

/**
 * smtpConfigDigest 将现有 SMTP 配置拼成参与 TXT 验证的摘要输入。
 *
 * 参数：
 * - cfg：YAML 中的 smtp 配置。
 * 返回值：稳定拼接后的 SMTP 配置摘要输入。
 * 失败条件：无。
 */
func smtpConfigDigest(cfg config.SMTPConfig) string {
	return cfg.Addr + cfg.Hostname
}
