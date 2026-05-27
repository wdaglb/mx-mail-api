package service

import (
	"context"
	"mime"
	"net/mail"
	"strings"

	"mx-mail-api/internal/mailparse"
	"mx-mail-api/internal/repository"
	"mx-mail-api/internal/storage"
)

/**
 * MessageService 承载收件记录可见性、正文解析和 SMTP 入库业务。
 *
 * 字段：
 * - messages：收件记录仓储。
 * - domains：域名仓储。
 */
type MessageService struct {
	messages  *repository.MessageRepository
	domains   *repository.DomainRepository
	temporary *TemporaryMailboxService
}

/**
 * NewMessageService 创建收件记录服务。
 *
 * 参数：
 * - messages：收件记录仓储。
 * - domains：域名仓储。
 * - temporary：临时邮箱服务，用于 SMTP 收件前校验地址是否已申请且未过期。
 * 返回值：收件记录服务实例。
 * 失败条件：无。
 */
func NewMessageService(messages *repository.MessageRepository, domains *repository.DomainRepository, temporary *TemporaryMailboxService) *MessageService {
	return &MessageService{messages: messages, domains: domains, temporary: temporary}
}

/**
 * ListVisible 返回当前用户可见的收件记录。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * 返回值：当前用户可见的收件记录。
 * 失败条件：邮件或域名查询失败时返回错误。
 */
func (service *MessageService) ListVisible(ctx context.Context, user storage.User) ([]storage.Message, error) {
	messages, err := service.messages.ListDesc(ctx)
	if err != nil {
		return nil, err
	}

	domains, err := service.domains.ListOwnedOrGlobal(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	return FilterMessagesByDomains(messages, domains), nil
}

/**
 * GetBody 返回当前用户可见邮件的正文详情。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * - id：收件记录 ID。
 * 返回值：原始邮件和解码后的正文。
 * 失败条件：邮件不存在、权限不足、域名查询失败时返回错误。
 */
func (service *MessageService) GetBody(ctx context.Context, user storage.User, id uint) (MessageBody, error) {
	message, err := service.messages.FindByID(ctx, id)
	if err != nil {
		return MessageBody{}, err
	}

	domains, err := service.domains.ListOwnedOrGlobal(ctx, user.ID)
	if err != nil {
		return MessageBody{}, err
	}
	if !MessageMatchesDomains(message, domains) {
		return MessageBody{}, ErrForbidden
	}

	return MessageBody{
		Message: message,
		Decoded: mailparse.Decode(message.Data),
	}, nil
}

/**
 * LatestForTemporaryMailbox 返回当前用户租赁邮箱收到的最新邮件。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * - address：租赁的临时邮箱地址。
 * 返回值：最新邮件摘要。
 * 失败条件：临时邮箱不存在、归属不匹配或没有邮件时返回错误。
 */
func (service *MessageService) LatestForTemporaryMailbox(ctx context.Context, user storage.User, address string) (OpenAPILatestMessage, error) {
	normalizedAddress := strings.ToLower(strings.TrimSpace(address))
	mailbox, err := service.temporary.FindOwned(ctx, user, normalizedAddress)
	if err != nil {
		return OpenAPILatestMessage{}, err
	}

	message, err := service.messages.LatestByRecipient(ctx, mailbox.Address)
	if err != nil {
		return OpenAPILatestMessage{}, err
	}

	return toOpenAPILatestMessage(message), nil
}

/**
 * SaveReceived 保存 SMTP 已接受邮件。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - input：SMTP 层提交的邮件数据。
 * 返回值：已保存邮件。
 * 失败条件：数据库拒绝插入时返回错误。
 */
func (service *MessageService) SaveReceived(ctx context.Context, input ReceivedMessageInput) (storage.Message, error) {
	return service.messages.Create(ctx, storage.Message{
		HeloName:   input.HeloName,
		MailFrom:   input.MailFrom,
		RcptTo:     append([]string(nil), input.RcptTo...),
		Data:       input.Data,
		RemoteAddr: input.RemoteAddr,
	})
}

/**
 * AcceptsRecipient 检查 SMTP 收件人是否是已申请且未过期的临时邮箱。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - address：RCPT TO 中的邮箱地址。
 * 返回值：地址存在有效临时邮箱且域名被接受时返回 true。
 * 失败条件：临时邮箱不存在、临时邮箱过期或域名仓储查询失败时返回错误；邮箱格式错误按不匹配处理。
 */
func (service *MessageService) AcceptsRecipient(ctx context.Context, address string) (bool, error) {
	if service.temporary != nil {
		if err := service.temporary.EnsureReceivable(ctx, address); err != nil {
			return false, err
		}
	}

	domain, ok := EmailDomain(address)
	if !ok {
		return false, nil
	}

	patterns, err := service.domains.AcceptedPatterns(ctx)
	if err != nil {
		return false, err
	}

	for _, pattern := range patterns {
		if storage.DomainMatches(pattern, domain) {
			return true, nil
		}
	}

	return false, nil
}

/**
 * FilterMessagesByDomains 保留至少一个收件人匹配域名规则的邮件。
 *
 * 参数：
 * - messages：候选收件记录。
 * - domains：当前用户拥有或可用的域名规则。
 * 返回值：可见收件记录。
 * 失败条件：无。
 */
func FilterMessagesByDomains(messages []storage.Message, domains []storage.AcceptedDomain) []storage.Message {
	visible := make([]storage.Message, 0, len(messages))
	for _, message := range messages {
		if MessageMatchesDomains(message, domains) {
			visible = append(visible, message)
		}
	}

	return visible
}

/**
 * MessageMatchesDomains 检查单封邮件是否匹配任一域名规则。
 *
 * 参数：
 * - message：已存储的 SMTP 邮件。
 * - domains：域名规则。
 * 返回值：任一收件人匹配任一域名规则时返回 true。
 * 失败条件：无。
 */
func MessageMatchesDomains(message storage.Message, domains []storage.AcceptedDomain) bool {
	for _, recipient := range message.RcptTo {
		recipientDomain, ok := EmailDomain(recipient)
		if !ok {
			continue
		}
		for _, domain := range domains {
			if storage.DomainMatches(domain.Domain, recipientDomain) {
				return true
			}
		}
	}

	return false
}

/**
 * EmailDomain 从邮箱地址中提取归一化域名。
 *
 * 参数：
 * - address：邮箱地址。
 * 返回值：地址包含域名时返回归一化域名和 true。
 * 失败条件：无。
 */
func EmailDomain(address string) (string, bool) {
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return "", false
	}

	domain := storage.NormalizeDomain(address[at+1:])
	return domain, domain != ""
}

/**
 * toOpenAPILatestMessage 将完整收件记录转换为开放接口最新邮件摘要。
 *
 * 参数：
 * - message：已存储邮件。
 * 返回值：只包含发件人、主题、正文和时间的开放接口 DTO。
 * 失败条件：无；邮件头解析失败时会回退到 SMTP envelope 信息。
 */
func toOpenAPILatestMessage(message storage.Message) OpenAPILatestMessage {
	decoded := mailparse.Decode(message.Data)
	result := OpenAPILatestMessage{
		From:      message.MailFrom,
		Body:      decoded.Body,
		CreatedAt: message.CreatedAt,
	}

	raw, err := mail.ReadMessage(strings.NewReader(message.Data))
	if err != nil {
		return result
	}

	if from := strings.TrimSpace(raw.Header.Get("From")); from != "" {
		result.From = from
	}

	decoder := new(mime.WordDecoder)
	subject, err := decoder.DecodeHeader(raw.Header.Get("Subject"))
	if err != nil {
		subject = raw.Header.Get("Subject")
	}
	result.Subject = subject
	return result
}
