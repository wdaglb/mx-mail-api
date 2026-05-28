package storage

import (
	"errors"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

/**
 * Message 保存一封已接受的 SMTP 邮件。
 *
 * 字段：
 * - ID：数据库主键。
 * - HeloName：客户端通过 HELO/EHLO 发送的名称。
 * - MailFrom：MAIL FROM 命令中的发件人。
 * - RcptTo：通过 RCPT TO 命令接受的收件人列表。
 * - Data：完成 SMTP 点转义还原后的原始 DATA 内容。
 * - RemoteAddr：TCP 对端地址，便于后续审计和排查。
 * - CreatedAt：由 GORM 管理的入库时间。
 */
type Message struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	HeloName   string    `gorm:"size:255;not null" json:"helo_name"`
	MailFrom   string    `gorm:"size:512;not null" json:"mail_from"`
	RcptTo     []string  `gorm:"serializer:json;not null" json:"rcpt_to"`
	Data       string    `gorm:"type:text;not null" json:"data"`
	RemoteAddr string    `gorm:"size:255;not null" json:"remote_addr"`
	CreatedAt  time.Time `json:"created_at"`
}

/**
 * TemporaryMailbox 保存用户申请的邮箱地址。
 *
 * 字段：
 * - ID：数据库主键。
 * - Address：完整邮箱地址，使用唯一索引避免自动生成名称碰撞导致重复地址。
 * - LocalPart：自动生成的邮箱名称。
 * - Domain：用户申请时选择的可用域名。
 * - OwnerUserID：申请该邮箱的用户 ID。
 * - Owner：管理 API 使用的 GORM 用户关联。
 * - ExpiresAt：临时邮箱过期时间；永久邮箱保留远期时间用于兼容既有非空字段。
 * - IsPermanent：是否为永久邮箱；永久邮箱不参与过期判断。
 * - CreatedAt：由 GORM 管理的创建时间。
 */
type TemporaryMailbox struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Address     string    `gorm:"size:512;uniqueIndex;not null" json:"address"`
	LocalPart   string    `gorm:"size:128;not null" json:"local_part"`
	Domain      string    `gorm:"size:255;not null;index" json:"domain"`
	OwnerUserID uint      `gorm:"not null;index" json:"owner_user_id"`
	Owner       *User     `gorm:"foreignKey:OwnerUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"owner"`
	ExpiresAt   time.Time `gorm:"not null;index" json:"expires_at"`
	IsPermanent bool      `gorm:"not null;default:false;index" json:"is_permanent"`
	CreatedAt   time.Time `json:"created_at"`
}

/**
 * User 保存一个本地登录账号。
 *
 * 字段：
 * - ID：数据库主键。
 * - Username：唯一登录用户名。
 * - PasswordHash：bcrypt 哈希后的密码。
 * - APIKeyHash：可选的 bcrypt 哈希 API Key；明文只会在生成或重置时返回一次。
 * - Role：授权角色，当前为 admin 或 user。
 * - TemporaryMailboxTTLMinutes：管理员配置的临时邮箱可选租赁分钟数；为空时由 service 层兜底为默认 30 分钟。
 * - CanLeasePermanentMailbox：是否允许申请永久邮箱；管理员初始化默认开启，普通用户由管理员配置。
 * - OpenAIQoSRPM：OpenAI 每分钟请求数上限；为空或 0 时由 service 层兜底为默认值。
 * - CreatedAt/UpdatedAt：由 GORM 管理的时间戳。
 */
type User struct {
	ID                         uint      `gorm:"primaryKey" json:"id"`
	Username                   string    `gorm:"size:128;uniqueIndex;not null" json:"username"`
	PasswordHash               string    `gorm:"size:255;not null" json:"-"`
	APIKeyHash                 string    `gorm:"size:255" json:"-"`
	Role                       string    `gorm:"size:32;not null;index" json:"role"`
	TemporaryMailboxTTLMinutes []int     `gorm:"serializer:json" json:"temporary_mailbox_ttl_minutes"`
	CanLeasePermanentMailbox   bool      `gorm:"not null;default:false" json:"can_lease_permanent_mailbox"`
	OpenAIQoSRPM               int       `gorm:"not null;default:60" json:"openai_qos_rpm"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

/**
 * AcceptedDomain 保存一个收件域名规则。
 *
 * 字段：
 * - ID：数据库主键。
 * - Domain：小写根域名；匹配该根域名和所有子域名，不再支持 "*" 通配写法。
 * - OwnerUserID：创建并拥有该域名的可选用户 ID；nil 表示全局域名，所有用户都可使用。
 * - Owner：管理 API 使用的可选 GORM 关联。
 * - Disabled：禁用后不再用于邮箱申请、SMTP 收件和收件记录可见性。
 * - MailboxQuota：该域名累计可创建的邮箱账号数量；0 表示不限额。
 * - CreatedAt/UpdatedAt：由 GORM 管理的时间戳。
 */
type AcceptedDomain struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Domain       string    `gorm:"size:255;uniqueIndex;not null" json:"domain"`
	OwnerUserID  *uint     `gorm:"index" json:"owner_user_id"`
	Owner        *User     `gorm:"foreignKey:OwnerUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"owner"`
	Disabled     bool      `gorm:"not null;default:false;index" json:"disabled"`
	MailboxQuota int       `gorm:"not null;default:0" json:"mailbox_quota"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

/**
 * GormMessageStore 持有 GORM 连接并负责初始化数据库 schema。
 *
 * 字段：
 * - db：已初始化的 GORM 数据库句柄。
 */
type GormMessageStore struct {
	db *gorm.DB
}

/**
 * OpenPostgresStore 连接 Postgres 并准备邮件相关数据表。
 *
 * 参数：
 * - dsn：基于 pgx 的 GORM 驱动接受的 Postgres DSN。
 * 返回值：由 Postgres 支撑的数据库连接包装。
 * 失败条件：dsn 为空、Postgres 不可达、认证失败，或 AutoMigrate 无法创建/更新 schema 时返回错误。
 */
func OpenPostgresStore(dsn string) (*GormMessageStore, error) {
	if dsn == "" {
		return nil, errors.New("DATABASE_DSN is required for Postgres message storage")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// AutoMigrate 被本项目选为 MVP 建表方式；生产环境如需严格变更审计，应后续替换为显式 migration。
	if err := db.AutoMigrate(&User{}, &AcceptedDomain{}, &TemporaryMailbox{}, &Message{}); err != nil {
		return nil, err
	}

	return &GormMessageStore{db: db}, nil
}

/**
 * DB 返回底层 GORM 句柄，供 API 仓储和初始化流程使用。
 *
 * 参数：无。
 * 返回值：已初始化的 GORM 数据库句柄。
 * 失败条件：无。
 */
func (store *GormMessageStore) DB() *gorm.DB {
	return store.db
}

/**
 * NormalizeDomain 规范化收件域名规则。
 *
 * 参数：
 * - domain：用户输入的域名。
 * 返回值：归一化后的小写域名规则。
 * 失败条件：无；结构校验单独处理，便于 API 区分归一化和非法输入。
 */
func NormalizeDomain(domain string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
}

/**
 * DomainMatches 检查收件域名规则是否匹配。
 *
 * 参数：
 * - pattern：已接受的根域名规则，例如 "example.com"。
 * - domain：已归一化的收件人域名。
 * 返回值：域名被规则接受时返回 true。
 * 失败条件：无；非法或空规则只会返回不匹配。
 */
func DomainMatches(pattern string, domain string) bool {
	normalized := NormalizeDomain(pattern)
	normalizedDomain := NormalizeDomain(domain)
	if normalized == "" || normalizedDomain == "" {
		return false
	}
	if strings.Contains(normalized, "*") {
		// 2026-05-27 起不再兼容旧的 "*.example.com" 通配写法，避免配置语义和临时邮箱地址生成出现分歧。
		return false
	}

	return normalizedDomain == normalized || strings.HasSuffix(normalizedDomain, "."+normalized)
}

/**
 * IsValidDomainPattern 检查域名规则是否可用于 RCPT TO 匹配。
 *
 * 参数：
 * - domain：根域名规则。
 * 返回值：规则存在非空根域名且不包含空白字符时返回 true。
 * 失败条件：无。
 */
func IsValidDomainPattern(domain string) bool {
	normalized := NormalizeDomain(domain)
	if normalized == "" || strings.ContainsAny(normalized, " \t\r\n") {
		return false
	}

	return !strings.Contains(normalized, "*")
}
