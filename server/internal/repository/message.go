package repository

import (
	"context"
	"strings"

	"mx-mail-api/internal/storage"

	"gorm.io/gorm"
)

/**
 * MessageRepository 封装收件记录表读写。
 *
 * 字段：
 * - db：已初始化的 GORM 数据库句柄。
 */
type MessageRepository struct {
	db *gorm.DB
}

/**
 * NewMessageRepository 创建收件记录仓储。
 *
 * 参数：
 * - db：已初始化的 GORM 数据库句柄。
 * 返回值：收件记录仓储实例。
 * 失败条件：无。
 */
func NewMessageRepository(db *gorm.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

/**
 * Create 插入收件记录。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - message：待保存的收件记录。
 * 返回值：已保存收件记录。
 * 失败条件：数据库拒绝插入时返回错误。
 */
func (repo *MessageRepository) Create(ctx context.Context, message storage.Message) (storage.Message, error) {
	err := repo.db.WithContext(ctx).Create(&message).Error
	return message, err
}

/**
 * ListDesc 按 ID 倒序列出收件记录。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * 返回值：收件记录列表。
 * 失败条件：数据库查询失败时返回错误。
 */
func (repo *MessageRepository) ListDesc(ctx context.Context) ([]storage.Message, error) {
	var messages []storage.Message
	err := repo.db.WithContext(ctx).Order("id DESC").Find(&messages).Error
	return messages, err
}

/**
 * FindByID 按 ID 查询收件记录。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - id：收件记录 ID。
 * 返回值：匹配收件记录。
 * 失败条件：记录不存在或数据库查询失败时返回错误。
 */
func (repo *MessageRepository) FindByID(ctx context.Context, id uint) (storage.Message, error) {
	var message storage.Message
	err := repo.db.WithContext(ctx).First(&message, id).Error
	return message, err
}

/**
 * LatestByRecipient 按收件地址查询最新一封收件记录。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - address：完整收件邮箱地址。
 * 返回值：包含该收件地址的最新邮件。
 * 失败条件：记录不存在或数据库查询失败时返回错误。
 */
func (repo *MessageRepository) LatestByRecipient(ctx context.Context, address string) (storage.Message, error) {
	var messages []storage.Message
	// RcptTo 使用 GORM JSON serializer 存储；MVP 阶段采用候选扫描，确保 Postgres 与测试 SQLite 行为一致。
	if err := repo.db.WithContext(ctx).Order("id DESC").Find(&messages).Error; err != nil {
		return storage.Message{}, err
	}
	normalizedAddress := strings.ToLower(strings.TrimSpace(address))
	for _, item := range messages {
		for _, recipient := range item.RcptTo {
			if strings.ToLower(strings.TrimSpace(recipient)) == normalizedAddress {
				return item, nil
			}
		}
	}

	return storage.Message{}, gorm.ErrRecordNotFound
}
