package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"mx-mail-api/internal/auth"
	"mx-mail-api/internal/config"
	"mx-mail-api/internal/repository"
	"mx-mail-api/internal/storage"
)

const (
	defaultTemporaryMailboxTTLMinutes = 30
	minTemporaryMailboxTTLMinutes     = 1
	maxTemporaryMailboxTTLMinutes     = 10080
	defaultOpenAIQoSRPM               = 60
	minOpenAIQoSRPM                   = 1
	maxOpenAIQoSRPM                   = 100000
)

/**
 * UserService 承载用户、登录和 API Key 相关业务。
 *
 * 字段：
 * - users：用户仓储。
 * - cfg：认证和初始化所需配置。
 */
type UserService struct {
	users *repository.UserRepository
	cfg   config.Config
}

/**
 * NewUserService 创建用户服务。
 *
 * 参数：
 * - users：用户仓储。
 * - cfg：运行时配置。
 * 返回值：用户服务实例。
 * 失败条件：无。
 */
func NewUserService(users *repository.UserRepository, cfg config.Config) *UserService {
	return &UserService{users: users, cfg: cfg}
}

/**
 * BootstrapInitialAdmin 在用户表为空时创建配置中的管理员。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * 返回值：初始化成功或无需初始化时返回 nil。
 * 失败条件：查询用户数、密码哈希或创建用户失败时返回错误。
 */
func (service *UserService) BootstrapInitialAdmin(ctx context.Context) error {
	count, err := service.users.Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := auth.HashPassword(service.cfg.Admin.InitialUser.Password)
	if err != nil {
		return err
	}

	_, err = service.users.Create(ctx, storage.User{
		Username:                   service.cfg.Admin.InitialUser.Username,
		PasswordHash:               hash,
		Role:                       storage.RoleAdmin,
		TemporaryMailboxTTLMinutes: DefaultTemporaryMailboxTTLMinutes(),
		CanLeasePermanentMailbox:   true,
		OpenAIQoSRPM:               defaultOpenAIQoSRPM,
	})
	return err
}

/**
 * Login 校验用户名密码并签发 JWT。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - username：登录用户名。
 * - password：登录密码。
 * 返回值：JWT token 和用户模型。
 * 失败条件：用户不存在、密码错误或 token 签发失败时返回错误。
 */
func (service *UserService) Login(ctx context.Context, username string, password string) (string, storage.User, error) {
	user, err := service.users.FindByUsername(ctx, username)
	if err != nil {
		return "", storage.User{}, ErrInvalidCredentials
	}
	if !auth.CheckPassword(user.PasswordHash, password) {
		return "", storage.User{}, ErrInvalidCredentials
	}

	token, err := auth.IssueToken(user, service.cfg.Auth.JWTSecret, time.Duration(service.cfg.Auth.TokenTTLHours)*time.Hour)
	if err != nil {
		return "", storage.User{}, err
	}

	return token, user, nil
}

/**
 * ResolveToken 将 JWT 或 API Key 解析为用户。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - token：Bearer token 或 X-API-Key 值。
 * 返回值：认证用户。
 * 失败条件：JWT 与 API Key 均无法匹配用户时返回错误。
 */
func (service *UserService) ResolveToken(ctx context.Context, token string) (storage.User, error) {
	claims, err := auth.ParseToken(token, service.cfg.Auth.JWTSecret)
	if err == nil {
		return service.users.FindByID(ctx, claims.UserID)
	}

	users, err := service.users.ListWithAPIKeyHash(ctx)
	if err != nil {
		return storage.User{}, err
	}
	for _, user := range users {
		if auth.CheckAPIKey(user.APIKeyHash, token) {
			return user, nil
		}
	}

	return storage.User{}, errors.New("api key not found")
}

/**
 * ResetAPIKey 为用户生成新的 API Key。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - user：当前用户。
 * 返回值：一次性明文 token 和更新后用户。
 * 失败条件：随机 token 生成、哈希或保存用户失败时返回错误。
 */
func (service *UserService) ResetAPIKey(ctx context.Context, user storage.User) (APIKeyResult, error) {
	token, err := auth.GenerateAPIKey()
	if err != nil {
		return APIKeyResult{}, err
	}

	hash, err := auth.HashAPIKey(token)
	if err != nil {
		return APIKeyResult{}, err
	}

	user.APIKeyHash = hash
	updated, err := service.users.Save(ctx, user)
	if err != nil {
		return APIKeyResult{}, err
	}

	return APIKeyResult{Token: token, User: updated}, nil
}

/**
 * ListUsers 返回全部用户。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * 返回值：用户列表。
 * 失败条件：数据库查询失败时返回错误。
 */
func (service *UserService) ListUsers(ctx context.Context) ([]storage.User, error) {
	return service.users.List(ctx)
}

/**
 * CreateUser 创建本地用户。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - username：用户名。
 * - password：明文密码。
 * - role：角色。
 * - ttlMinutes：管理员配置的临时邮箱可选租赁分钟数；为空时使用默认 30 分钟。
 * - canLeasePermanentMailbox：是否允许该用户申请永久邮箱。
 * - openAIQoSRPM：OpenAI 每分钟请求数上限；0 表示使用默认值。
 * 返回值：已创建用户。
 * 失败条件：参数非法、密码哈希或创建失败时返回错误。
 */
func (service *UserService) CreateUser(ctx context.Context, username string, password string, role string, ttlMinutes []int, canLeasePermanentMailbox bool, openAIQoSRPM int) (storage.User, error) {
	normalizedRole, ok := NormalizeRole(role)
	if !ok || strings.TrimSpace(username) == "" || password == "" {
		return storage.User{}, ErrInvalidUser
	}

	normalizedTTLMinutes, err := NormalizeTemporaryMailboxTTLMinutes(ttlMinutes)
	if err != nil {
		return storage.User{}, err
	}
	normalizedOpenAIQoSRPM, err := NormalizeOpenAIQoSRPM(openAIQoSRPM)
	if err != nil {
		return storage.User{}, err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return storage.User{}, err
	}

	return service.users.Create(ctx, storage.User{
		Username:                   strings.TrimSpace(username),
		PasswordHash:               hash,
		Role:                       normalizedRole,
		TemporaryMailboxTTLMinutes: normalizedTTLMinutes,
		CanLeasePermanentMailbox:   canLeasePermanentMailbox || normalizedRole == storage.RoleAdmin,
		OpenAIQoSRPM:               normalizedOpenAIQoSRPM,
	})
}

/**
 * UpdateUser 更新用户资料。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - id：用户 ID。
 * - username：可选用户名。
 * - password：可选明文密码。
 * - role：可选角色。
 * - ttlMinutes：可选临时邮箱租赁分钟数；nil 表示本次不修改。
 * - canLeasePermanentMailbox：可选永久邮箱申请权限；nil 表示本次不修改。
 * - openAIQoSRPM：可选 OpenAI 每分钟请求数上限；nil 表示本次不修改。
 * 返回值：更新后用户。
 * 失败条件：用户不存在、角色非法、密码哈希或保存失败时返回错误。
 */
func (service *UserService) UpdateUser(ctx context.Context, id uint, username string, password string, role string, ttlMinutes []int, canLeasePermanentMailbox *bool, openAIQoSRPM *int) (storage.User, error) {
	user, err := service.users.FindByID(ctx, id)
	if err != nil {
		return storage.User{}, err
	}

	if trimmed := strings.TrimSpace(username); trimmed != "" {
		user.Username = trimmed
	}
	if role != "" {
		normalizedRole, ok := NormalizeRole(role)
		if !ok {
			return storage.User{}, ErrInvalidRole
		}
		user.Role = normalizedRole
	}
	if password != "" {
		hash, err := auth.HashPassword(password)
		if err != nil {
			return storage.User{}, err
		}
		user.PasswordHash = hash
	}
	if ttlMinutes != nil {
		normalizedTTLMinutes, err := NormalizeTemporaryMailboxTTLMinutes(ttlMinutes)
		if err != nil {
			return storage.User{}, err
		}
		user.TemporaryMailboxTTLMinutes = normalizedTTLMinutes
	}
	if canLeasePermanentMailbox != nil {
		user.CanLeasePermanentMailbox = *canLeasePermanentMailbox || user.Role == storage.RoleAdmin
	}
	if openAIQoSRPM != nil {
		normalizedOpenAIQoSRPM, err := NormalizeOpenAIQoSRPM(*openAIQoSRPM)
		if err != nil {
			return storage.User{}, err
		}
		user.OpenAIQoSRPM = normalizedOpenAIQoSRPM
	}

	return service.users.Save(ctx, user)
}

/**
 * DeleteUser 删除用户。
 *
 * 参数：
 * - ctx：业务操作上下文。
 * - id：用户 ID。
 * 返回值：无。
 * 失败条件：数据库删除失败时返回错误。
 */
func (service *UserService) DeleteUser(ctx context.Context, id uint) error {
	return service.users.Delete(ctx, id)
}

/**
 * ToUserDTO 将用户模型转换为 service 层 DTO。
 *
 * 参数：
 * - user：用户模型。
 * 返回值：用户 DTO。
 * 失败条件：无。
 */
func ToUserDTO(user storage.User) UserDTO {
	return UserDTO{
		ID:                         user.ID,
		Username:                   user.Username,
		Role:                       user.Role,
		HasAPIKey:                  user.APIKeyHash != "",
		TemporaryMailboxTTLMinutes: AllowedTemporaryMailboxTTLMinutes(user),
		CanLeasePermanentMailbox:   CanLeasePermanentMailbox(user),
		OpenAIQoSRPM:               OpenAIQoSRPM(user),
		CreatedAt:                  user.CreatedAt,
		UpdatedAt:                  user.UpdatedAt,
	}
}

/**
 * CanLeasePermanentMailbox 返回用户是否可申请永久邮箱。
 *
 * 参数：
 * - user：用户模型。
 * 返回值：管理员始终可申请永久邮箱；普通用户按管理员配置返回。
 * 失败条件：无。
 */
func CanLeasePermanentMailbox(user storage.User) bool {
	return user.Role == storage.RoleAdmin || user.CanLeasePermanentMailbox
}

/**
 * OpenAIQoSRPM 返回用户 OpenAI 每分钟请求数上限。
 *
 * 参数：
 * - user：用户模型。
 * 返回值：用户已配置 RPM；为空或 0 时返回默认值。
 * 失败条件：无。
 */
func OpenAIQoSRPM(user storage.User) int {
	if user.OpenAIQoSRPM <= 0 {
		return defaultOpenAIQoSRPM
	}

	return user.OpenAIQoSRPM
}

/**
 * NormalizeRole 校验并归一化角色。
 *
 * 参数：
 * - role：用户输入角色。
 * 返回值：归一化角色和是否有效。
 * 失败条件：无。
 */
func NormalizeRole(role string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(role))
	return normalized, normalized == storage.RoleAdmin || normalized == storage.RoleUser
}

/**
 * DefaultTemporaryMailboxTTLMinutes 返回系统默认临时邮箱租赁分钟数。
 *
 * 参数：无。
 * 返回值：新的默认租赁分钟数切片。
 * 失败条件：无。
 */
func DefaultTemporaryMailboxTTLMinutes() []int {
	return []int{defaultTemporaryMailboxTTLMinutes}
}

/**
 * AllowedTemporaryMailboxTTLMinutes 返回用户可选择的临时邮箱租赁分钟数。
 *
 * 参数：
 * - user：当前用户模型。
 * 返回值：用户已配置租赁分钟数；为空时返回默认 30 分钟。
 * 失败条件：无。
 */
func AllowedTemporaryMailboxTTLMinutes(user storage.User) []int {
	if len(user.TemporaryMailboxTTLMinutes) == 0 {
		return DefaultTemporaryMailboxTTLMinutes()
	}

	// 返回副本，避免调用方误改 GORM 模型字段导致后续保存出现隐式副作用。
	values := make([]int, len(user.TemporaryMailboxTTLMinutes))
	copy(values, user.TemporaryMailboxTTLMinutes)
	return values
}

/**
 * NormalizeTemporaryMailboxTTLMinutes 归一化管理员配置的临时邮箱租赁分钟数。
 *
 * 参数：
 * - values：管理员提交的分钟数列表。
 * 返回值：去重、升序后的分钟数；空列表按兼容规则归一化为默认 30 分钟。
 * 失败条件：存在小于 1 或大于 10080 的分钟数时返回 ErrInvalidTemporaryMailboxTTL。
 */
func NormalizeTemporaryMailboxTTLMinutes(values []int) ([]int, error) {
	if len(values) == 0 {
		return DefaultTemporaryMailboxTTLMinutes(), nil
	}

	unique := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value < minTemporaryMailboxTTLMinutes || value > maxTemporaryMailboxTTLMinutes {
			return nil, ErrInvalidTemporaryMailboxTTL
		}
		unique[value] = struct{}{}
	}

	normalized := make([]int, 0, len(unique))
	for value := range unique {
		normalized = append(normalized, value)
	}
	sort.Ints(normalized)
	return normalized, nil
}

/**
 * ResolveTemporaryMailboxTTLMinutes 解析一次临时邮箱申请最终使用的租赁分钟数。
 *
 * 参数：
 * - user：当前申请用户。
 * - requested：请求体中的 ttl_minutes；nil 表示旧客户端未传该字段。
 * 返回值：本次申请应使用的租赁分钟数。
 * 失败条件：请求值不在用户允许列表内时返回 ErrInvalidTemporaryMailboxTTL。
 */
func ResolveTemporaryMailboxTTLMinutes(user storage.User, requested *int) (int, error) {
	allowed := AllowedTemporaryMailboxTTLMinutes(user)
	if len(allowed) == 0 {
		// 该分支仅作为防御性兜底；AllowedTemporaryMailboxTTLMinutes 理论上不会返回空列表。
		return defaultTemporaryMailboxTTLMinutes, nil
	}
	if requested == nil {
		return allowed[0], nil
	}
	for _, value := range allowed {
		if value == *requested {
			return value, nil
		}
	}

	return 0, ErrInvalidTemporaryMailboxTTL
}

/**
 * NormalizeOpenAIQoSRPM 归一化 OpenAI QoS RPM 配置。
 *
 * 参数：
 * - value：管理员提交的每分钟请求数上限；0 表示使用默认值。
 * 返回值：可保存的 RPM 值。
 * 失败条件：小于 0 或超过系统上限时返回 ErrInvalidOpenAIQoS。
 */
func NormalizeOpenAIQoSRPM(value int) (int, error) {
	if value == 0 {
		return defaultOpenAIQoSRPM, nil
	}
	if value < minOpenAIQoSRPM || value > maxOpenAIQoSRPM {
		return 0, ErrInvalidOpenAIQoS
	}

	return value, nil
}
