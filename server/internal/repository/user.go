package repository

import (
	"context"

	"mx-mail-api/internal/storage"

	"gorm.io/gorm"
)

/**
 * UserRepository 封装用户表读写，避免 API 和 service 直接散落 GORM 查询。
 *
 * 字段：
 * - db：已初始化的 GORM 数据库句柄。
 */
type UserRepository struct {
	db *gorm.DB
}

/**
 * NewUserRepository 创建用户仓储。
 *
 * 参数：
 * - db：已初始化的 GORM 数据库句柄。
 * 返回值：用户仓储实例。
 * 失败条件：无。
 */
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

/**
 * Count 统计用户数量。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * 返回值：用户数量。
 * 失败条件：数据库查询失败时返回错误。
 */
func (repo *UserRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := repo.db.WithContext(ctx).Model(&storage.User{}).Count(&count).Error
	return count, err
}

/**
 * FindByID 按 ID 查询用户。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - id：用户 ID。
 * 返回值：匹配用户。
 * 失败条件：用户不存在或数据库查询失败时返回错误。
 */
func (repo *UserRepository) FindByID(ctx context.Context, id uint) (storage.User, error) {
	var user storage.User
	err := repo.db.WithContext(ctx).First(&user, id).Error
	return user, err
}

/**
 * FindByUsername 按用户名查询用户。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - username：登录用户名。
 * 返回值：匹配用户。
 * 失败条件：用户不存在或数据库查询失败时返回错误。
 */
func (repo *UserRepository) FindByUsername(ctx context.Context, username string) (storage.User, error) {
	var user storage.User
	err := repo.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	return user, err
}

/**
 * List 按 ID 升序列出全部用户。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * 返回值：用户列表。
 * 失败条件：数据库查询失败时返回错误。
 */
func (repo *UserRepository) List(ctx context.Context) ([]storage.User, error) {
	var users []storage.User
	err := repo.db.WithContext(ctx).Order("id ASC").Find(&users).Error
	return users, err
}

/**
 * ListWithAPIKeyHash 列出已配置 API Key 哈希的用户。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * 返回值：存在 API Key 哈希的用户列表。
 * 失败条件：数据库查询失败时返回错误。
 */
func (repo *UserRepository) ListWithAPIKeyHash(ctx context.Context) ([]storage.User, error) {
	var users []storage.User
	err := repo.db.WithContext(ctx).Where("api_key_hash <> ''").Find(&users).Error
	return users, err
}

/**
 * Create 插入用户。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - user：待创建用户。
 * 返回值：已创建用户。
 * 失败条件：数据库拒绝插入时返回错误。
 */
func (repo *UserRepository) Create(ctx context.Context, user storage.User) (storage.User, error) {
	err := repo.db.WithContext(ctx).Create(&user).Error
	return user, err
}

/**
 * Save 保存用户完整模型。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - user：待保存用户。
 * 返回值：已保存用户。
 * 失败条件：数据库拒绝更新时返回错误。
 */
func (repo *UserRepository) Save(ctx context.Context, user storage.User) (storage.User, error) {
	err := repo.db.WithContext(ctx).Save(&user).Error
	return user, err
}

/**
 * Delete 按 ID 删除用户。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - id：用户 ID。
 * 返回值：无。
 * 失败条件：数据库删除失败时返回错误。
 */
func (repo *UserRepository) Delete(ctx context.Context, id uint) error {
	return repo.db.WithContext(ctx).Delete(&storage.User{}, id).Error
}
