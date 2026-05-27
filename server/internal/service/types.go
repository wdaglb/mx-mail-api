package service

import (
	"time"

	"mx-mail-api/internal/mailparse"
	"mx-mail-api/internal/storage"
)

/**
 * APIKeyResult 表示 API Key 重置后的业务结果。
 *
 * 字段：
 * - Token：只展示一次的明文 API Key。
 * - User：更新后的用户模型。
 */
type APIKeyResult struct {
	Token string
	User  storage.User
}

/**
 * MessageBody 表示收件记录正文详情。
 *
 * 字段：
 * - Message：原始收件记录模型。
 * - Decoded：解码后的正文变体。
 */
type MessageBody struct {
	Message storage.Message
	Decoded mailparse.DecodedBody
}

/**
 * OpenAPILatestMessage 表示开放接口返回的最新邮件摘要。
 *
 * 字段：
 * - From：邮件 From 头或 SMTP MAIL FROM 兜底。
 * - Subject：解码后的邮件主题。
 * - Body：解码后的正文。
 * - CreatedAt：邮件入库时间。
 */
type OpenAPILatestMessage struct {
	From      string
	Subject   string
	Body      string
	CreatedAt time.Time
}

/**
 * ReceivedMessageInput 表示 SMTP 层提交给业务层的入库数据。
 *
 * 字段：
 * - HeloName：SMTP HELO/EHLO 名称。
 * - MailFrom：MAIL FROM 发件人。
 * - RcptTo：已接受的 RCPT TO 收件人列表。
 * - Data：已完成点转义还原后的 DATA 内容。
 * - RemoteAddr：TCP 对端地址。
 */
type ReceivedMessageInput struct {
	HeloName   string
	MailFrom   string
	RcptTo     []string
	Data       string
	RemoteAddr string
}

/**
 * UserDTO 表示 service 对 API 层输出的用户数据。
 *
 * 字段：
 * - ID：用户 ID。
 * - Username：用户名。
 * - Role：角色。
 * - HasAPIKey：是否已配置 API Key。
 * - TemporaryMailboxTTLMinutes：用户可选择的临时邮箱租赁分钟数。
 * - CanLeasePermanentMailbox：用户是否可以申请永久邮箱。
 * - OpenAIQoSRPM：OpenAI 每分钟请求数上限。
 * - CreatedAt/UpdatedAt：创建和更新时间。
 */
type UserDTO struct {
	ID                         uint
	Username                   string
	Role                       string
	HasAPIKey                  bool
	TemporaryMailboxTTLMinutes []int
	CanLeasePermanentMailbox   bool
	OpenAIQoSRPM               int
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

/**
 * TemporaryMailboxResult 表示申请临时邮箱后的业务结果。
 *
 * 字段：
 * - Mailbox：已创建的临时邮箱。
 * - TTLMinutes：本次申请使用的有效分钟数；永久邮箱为 0。
 */
type TemporaryMailboxResult struct {
	Mailbox    storage.TemporaryMailbox
	TTLMinutes int
}
