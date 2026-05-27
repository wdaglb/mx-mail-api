package repository

import (
	"context"
	"time"

	"mx-mail-api/internal/storage"

	"gorm.io/gorm"
)

/**
 * TemporaryMailboxRepository 封装临时邮箱表读写。
 *
 * 字段：
 * - db：已初始化的 GORM 数据库句柄。
 */
type TemporaryMailboxRepository struct {
	db *gorm.DB
}

/**
 * NewTemporaryMailboxRepository 创建临时邮箱仓储。
 *
 * 参数：
 * - db：已初始化的 GORM 数据库句柄。
 * 返回值：临时邮箱仓储实例。
 * 失败条件：无。
 */
func NewTemporaryMailboxRepository(db *gorm.DB) *TemporaryMailboxRepository {
	return &TemporaryMailboxRepository{db: db}
}

/**
 * Create 插入临时邮箱。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - mailbox：待创建临时邮箱。
 * 返回值：已创建临时邮箱。
 * 失败条件：地址重复或数据库拒绝插入时返回错误。
 */
func (repo *TemporaryMailboxRepository) Create(ctx context.Context, mailbox storage.TemporaryMailbox) (storage.TemporaryMailbox, error) {
	err := repo.db.WithContext(ctx).Create(&mailbox).Error
	return mailbox, err
}

/**
 * ListByOwner 按创建时间倒序列出用户可用邮箱。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - ownerID：用户 ID。
 * 返回值：用户申请过且尚未过期的临时邮箱，以及所有永久邮箱。
 * 失败条件：数据库查询失败时返回错误。
 */
func (repo *TemporaryMailboxRepository) ListByOwner(ctx context.Context, ownerID uint) ([]storage.TemporaryMailbox, error) {
	var mailboxes []storage.TemporaryMailbox
	err := repo.db.WithContext(ctx).
		Where("owner_user_id = ? AND (is_permanent = ? OR expires_at > ?)", ownerID, true, time.Now()).
		Order("id DESC").
		Find(&mailboxes).Error
	return mailboxes, err
}

/**
 * FindByAddress 按完整邮箱地址查询临时邮箱。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - address：完整邮箱地址。
 * 返回值：匹配的临时邮箱。
 * 失败条件：地址不存在或数据库查询失败时返回错误。
 */
func (repo *TemporaryMailboxRepository) FindByAddress(ctx context.Context, address string) (storage.TemporaryMailbox, error) {
	var mailbox storage.TemporaryMailbox
	err := repo.db.WithContext(ctx).Where("address = ?", address).First(&mailbox).Error
	return mailbox, err
}
