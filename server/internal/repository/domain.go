package repository

import (
	"context"

	"mx-mail-api/internal/storage"

	"gorm.io/gorm"
)

/**
 * DomainRepository 封装接受域名表读写。
 *
 * 字段：
 * - db：已初始化的 GORM 数据库句柄。
 */
type DomainRepository struct {
	db *gorm.DB
}

/**
 * NewDomainRepository 创建域名仓储。
 *
 * 参数：
 * - db：已初始化的 GORM 数据库句柄。
 * 返回值：域名仓储实例。
 * 失败条件：无。
 */
func NewDomainRepository(db *gorm.DB) *DomainRepository {
	return &DomainRepository{db: db}
}

/**
 * ListVisible 返回当前用户可见的域名。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - user：当前用户。
 * 返回值：当前用户可见的域名列表。
 * 失败条件：数据库查询失败时返回错误。
 */
func (repo *DomainRepository) ListVisible(ctx context.Context, user storage.User) ([]storage.AcceptedDomain, error) {
	query := repo.db.WithContext(ctx).Preload("Owner").Order("id ASC")
	if user.Role != storage.RoleAdmin {
		query = query.Where("owner_user_id = ? OR owner_user_id IS NULL", user.ID)
	}

	var domains []storage.AcceptedDomain
	err := query.Find(&domains).Error
	return domains, err
}

/**
 * ListOwnedOrGlobal 返回用户拥有的域名和全局域名。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - userID：当前用户 ID。
 * 返回值：用户拥有或全局可用的域名列表。
 * 失败条件：数据库查询失败时返回错误。
 */
func (repo *DomainRepository) ListOwnedOrGlobal(ctx context.Context, userID uint) ([]storage.AcceptedDomain, error) {
	var domains []storage.AcceptedDomain
	err := repo.db.WithContext(ctx).Where("(owner_user_id = ? OR owner_user_id IS NULL) AND disabled = ?", userID, false).Find(&domains).Error
	return domains, err
}

/**
 * AcceptedPatterns 返回 SMTP 收件策略使用的全部域名规则。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * 返回值：已配置域名规则列表。
 * 失败条件：数据库查询失败时返回错误。
 */
func (repo *DomainRepository) AcceptedPatterns(ctx context.Context) ([]string, error) {
	var domains []storage.AcceptedDomain
	if err := repo.db.WithContext(ctx).Where("disabled = ?", false).Order("domain ASC").Find(&domains).Error; err != nil {
		return nil, err
	}

	patterns := make([]string, 0, len(domains))
	for _, domain := range domains {
		patterns = append(patterns, domain.Domain)
	}

	return patterns, nil
}

/**
 * FindByID 按 ID 查询域名。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - id：域名 ID。
 * 返回值：匹配域名。
 * 失败条件：域名不存在或数据库查询失败时返回错误。
 */
func (repo *DomainRepository) FindByID(ctx context.Context, id uint) (storage.AcceptedDomain, error) {
	var domain storage.AcceptedDomain
	err := repo.db.WithContext(ctx).First(&domain, id).Error
	return domain, err
}

/**
 * Create 插入接受域名。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - domain：待创建域名。
 * 返回值：已创建域名，并预加载 Owner。
 * 失败条件：数据库拒绝插入或回查失败时返回错误。
 */
func (repo *DomainRepository) Create(ctx context.Context, domain storage.AcceptedDomain) (storage.AcceptedDomain, error) {
	if err := repo.db.WithContext(ctx).Create(&domain).Error; err != nil {
		return storage.AcceptedDomain{}, err
	}

	err := repo.db.WithContext(ctx).Preload("Owner").First(&domain, domain.ID).Error
	return domain, err
}

/**
 * Save 保存接受域名。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - domain：待保存域名。
 * 返回值：已保存域名，并预加载 Owner。
 * 失败条件：数据库拒绝更新或回查失败时返回错误。
 */
func (repo *DomainRepository) Save(ctx context.Context, domain storage.AcceptedDomain) (storage.AcceptedDomain, error) {
	if err := repo.db.WithContext(ctx).Save(&domain).Error; err != nil {
		return storage.AcceptedDomain{}, err
	}

	err := repo.db.WithContext(ctx).Preload("Owner").First(&domain, domain.ID).Error
	return domain, err
}

/**
 * Delete 删除接受域名。
 *
 * 参数：
 * - ctx：数据库操作上下文。
 * - domain：待删除域名。
 * 返回值：无。
 * 失败条件：数据库删除失败时返回错误。
 */
func (repo *DomainRepository) Delete(ctx context.Context, domain storage.AcceptedDomain) error {
	return repo.db.WithContext(ctx).Delete(&domain).Error
}
