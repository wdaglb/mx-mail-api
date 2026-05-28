package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mx-mail-api/internal/config"
	"mx-mail-api/internal/service"
	"mx-mail-api/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

/**
 * Server 持有 HTTP API 依赖并注册 Gin 路由。
 *
 * 字段：
 * - users：用户业务服务。
 * - domains：域名业务服务。
 * - messages：收件记录业务服务。
 * - openAIQoS：OpenAI 单用户 RPM 限流器。
 * - cfg：进程配置快照，用于向前端输出非敏感展示配置。
 */
type Server struct {
	users     *service.UserService
	domains   *service.DomainService
	messages  *service.MessageService
	temporary *service.TemporaryMailboxService
	openAIQoS *service.OpenAIQoSLimiter
	cfg       config.Config
}

/**
 * New 创建 API 服务实例。
 *
 * 参数：
 * - users：用户业务服务。
 * - domains：域名业务服务。
 * - messages：收件记录业务服务。
 * - temporary：临时邮箱业务服务。
 * - cfg：进程配置快照。
 * 返回值：可注册路由的 API 服务。
 * 失败条件：无。
 */
func New(users *service.UserService, domains *service.DomainService, messages *service.MessageService, temporary *service.TemporaryMailboxService, cfg ...config.Config) *Server {
	serverConfig := config.Config{}
	if len(cfg) > 0 {
		serverConfig = cfg[0]
	}

	return &Server{users: users, domains: domains, messages: messages, temporary: temporary, openAIQoS: service.NewOpenAIQoSLimiter(), cfg: serverConfig}
}

/**
 * CheckOpenAIQoS 检查当前用户是否超过 OpenAI 单用户 RPM 限制。
 *
 * 参数：
 * - user：当前认证用户。
 * 返回值：允许调用时返回 nil；超过限制时返回 ErrOpenAIQoSExceeded。
 * 失败条件：无；未初始化限流器时会延迟初始化，避免测试构造 Server 时遗漏字段。
 */
func (server *Server) CheckOpenAIQoS(user storage.User) error {
	if server.openAIQoS == nil {
		server.openAIQoS = service.NewOpenAIQoSLimiter()
	}

	return server.openAIQoS.Allow(user)
}

/**
 * RegisterRoutes 将 API 路由挂载到 Gin 引擎。
 *
 * 参数：
 * - router：Gin 引擎。
 * 返回值：无。
 * 失败条件：无；路由注册是确定性的。
 */
func (server *Server) RegisterRoutes(router *gin.Engine) {
	router.GET("/docs/api.md", server.apiDocument)

	group := router.Group("/api")
	group.POST("/auth/login", server.login)
	group.GET("/public-config", server.publicConfig)

	authed := group.Group("")
	authed.Use(server.authMiddleware())
	authed.GET("/me", server.me)
	authed.POST("/me/api-key", server.resetMyAPIKey)
	authed.GET("/temporary-mailboxes", server.listTemporaryMailboxes)
	authed.POST("/temporary-mailboxes", server.createTemporaryMailbox)
	authed.GET("/domains", server.listDomains)
	authed.POST("/domains/verification", server.generateDomainVerification)
	authed.POST("/domains", server.createDomain)
	authed.PUT("/domains/:id", server.updateDomain)
	authed.DELETE("/domains/:id", server.deleteDomain)
	authed.GET("/messages", server.listMessages)
	authed.GET("/messages/:id/body", server.getMessageBody)

	openapi := router.Group("/openapi")
	openapi.Use(server.authMiddleware())
	openapi.POST("/temporary-mailboxes", server.createOpenAPITemporaryMailbox)
	openapi.GET("/temporary-mailboxes/latest-message", server.getOpenAPILatestMessage)

	admin := authed.Group("")
	admin.Use(requireAdmin)
	admin.GET("/users", server.listUsers)
	admin.POST("/users", server.createUser)
	admin.PUT("/users/:id", server.updateUser)
	admin.DELETE("/users/:id", server.deleteUser)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userRequest struct {
	Username                   string `json:"username"`
	Password                   string `json:"password"`
	Role                       string `json:"role"`
	TemporaryMailboxTTLMinutes []int  `json:"temporary_mailbox_ttl_minutes"`
	CanLeasePermanentMailbox   *bool  `json:"can_lease_permanent_mailbox"`
	OpenAIQoSRPM               *int   `json:"openai_qos_rpm"`
}

type domainRequest struct {
	Domain            string `json:"domain"`
	OwnerUserID       *uint  `json:"owner_user_id"`
	VerificationName  string `json:"verification_name"`
	VerificationValue string `json:"verification_value"`
	Disabled          *bool  `json:"disabled"`
	MailboxQuota      *int   `json:"mailbox_quota"`
}

type domainVerificationRequest struct {
	Domain string `json:"domain"`
}

type temporaryMailboxRequest struct {
	Domain     string `json:"domain"`
	LocalPart  string `json:"local_part"`
	TTLMinutes *int   `json:"ttl_minutes"`
	Permanent  bool   `json:"permanent"`
}

type userResponse struct {
	ID                         uint      `json:"id"`
	Username                   string    `json:"username"`
	Role                       string    `json:"role"`
	HasAPIKey                  bool      `json:"has_api_key"`
	TemporaryMailboxTTLMinutes []int     `json:"temporary_mailbox_ttl_minutes"`
	CanLeasePermanentMailbox   bool      `json:"can_lease_permanent_mailbox"`
	OpenAIQoSRPM               int       `json:"openai_qos_rpm"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type apiKeyResponse struct {
	Token string       `json:"token"`
	User  userResponse `json:"user"`
}

type domainResponse struct {
	ID           uint      `json:"id"`
	Domain       string    `json:"domain"`
	OwnerUserID  *uint     `json:"owner_user_id"`
	OwnerName    string    `json:"owner_name"`
	Disabled     bool      `json:"disabled"`
	MailboxQuota int       `json:"mailbox_quota"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type domainVerificationResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type messageResponse struct {
	ID         uint      `json:"id"`
	HeloName   string    `json:"helo_name"`
	MailFrom   string    `json:"mail_from"`
	RcptTo     []string  `json:"rcpt_to"`
	RemoteAddr string    `json:"remote_addr"`
	CreatedAt  time.Time `json:"created_at"`
}

type messageBodyResponse struct {
	ID     uint   `json:"id"`
	Data   string `json:"data"`
	Body   string `json:"body"`
	HTML   string `json:"html"`
	IsHTML bool   `json:"is_html"`
}

type temporaryMailboxResponse struct {
	ID          uint      `json:"id"`
	Address     string    `json:"address"`
	LocalPart   string    `json:"local_part"`
	Domain      string    `json:"domain"`
	OwnerUserID uint      `json:"owner_user_id"`
	ExpiresAt   time.Time `json:"expires_at"`
	IsPermanent bool      `json:"is_permanent"`
	CreatedAt   time.Time `json:"created_at"`
	Expired     bool      `json:"expired"`
}

type temporaryMailboxCreateResponse struct {
	temporaryMailboxResponse
	TTLMinutes int `json:"ttl_minutes"`
}

type openAPILatestMessageResponse struct {
	From      string    `json:"from"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type publicConfigResponse struct {
	SMTPHostname string `json:"smtp_hostname"`
}

/**
 * apiDocument 输出开放接口 Markdown 文档。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：根目录 API.md 的 text/markdown 内容。
 * 失败条件：文件不存在或读取失败时返回 404，避免公开接口页面展示过期副本。
 */
func (server *Server) apiDocument(ctx *gin.Context) {
	content, err := readAPIDocument()
	if err != nil {
		writeError(ctx, http.StatusNotFound, "not_found", "api document not found")
		return
	}

	ctx.Data(http.StatusOK, "text/markdown; charset=utf-8", content)
}

/**
 * publicConfig 返回前端可公开展示的非敏感运行配置。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：包含 SMTP 主机名的 JSON。
 * 失败条件：无；敏感配置不会通过该接口输出。
 */
func (server *Server) publicConfig(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"item": publicConfigResponse{
			SMTPHostname: server.cfg.SMTP.Hostname,
		},
	})
}

/**
 * readAPIDocument 读取项目根目录 API.md。
 *
 * 参数：无。
 * 返回值：API.md 文件内容。
 * 失败条件：当前进程既不在项目根目录，也不在 server 子目录启动，或文件不存在时返回错误。
 */
func readAPIDocument() ([]byte, error) {
	candidates := []string{
		"API.md",
		filepath.Join("..", "API.md"),
	}
	var lastErr error
	for _, candidate := range candidates {
		content, err := os.ReadFile(candidate)
		if err == nil {
			return content, nil
		}
		lastErr = err
	}

	return nil, lastErr
}

/**
 * login 认证本地用户并签发 JWT。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：包含 token 和用户数据的 JSON 响应。
 * 失败条件：JSON 格式错误返回 400，凭据无效返回 401，令牌签发失败返回 500。
 */
func (server *Server) login(ctx *gin.Context) {
	var req loginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeError(ctx, http.StatusBadRequest, "invalid_request", "invalid login payload")
		return
	}

	token, user, err := server.users.Login(ctx.Request.Context(), req.Username, req.Password)
	if errors.Is(err, service.ErrInvalidCredentials) {
		writeError(ctx, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "token_error", "failed to issue token")
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  toUserResponse(user),
	})
}

/**
 * me 返回当前认证用户资料。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：JSON 用户资料。
 * 失败条件：中间件认证状态缺失时返回 401。
 */
func (server *Server) me(ctx *gin.Context) {
	user, ok := currentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "unauthorized", "missing authenticated user")
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"user": toUserResponse(user)})
}

/**
 * resetMyAPIKey 为当前用户生成新的 API Key。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：只展示一次的明文 token，以及更新后的用户资料。
 * 失败条件：认证状态缺失返回 401；token 生成、哈希或用户保存失败返回 500。
 */
func (server *Server) resetMyAPIKey(ctx *gin.Context) {
	user, ok := currentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "unauthorized", "missing authenticated user")
		return
	}

	result, err := server.users.ResetAPIKey(ctx.Request.Context(), user)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "api_key_failed", "failed to reset api key")
		return
	}
	ctx.Set("user", result.User)

	ctx.JSON(http.StatusOK, gin.H{
		"item": apiKeyResponse{
			Token: result.Token,
			User:  toUserResponse(result.User),
		},
	})
}

/**
 * listTemporaryMailboxes 返回当前用户申请过且尚未过期的临时邮箱。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：临时邮箱 JSON 数组。
 * 失败条件：认证状态缺失返回 401，查询失败返回 500。
 */
func (server *Server) listTemporaryMailboxes(ctx *gin.Context) {
	user, ok := currentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "unauthorized", "missing authenticated user")
		return
	}

	items, err := server.temporary.ListByOwner(ctx.Request.Context(), user)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "query_failed", "failed to list temporary mailboxes")
		return
	}

	resp := make([]temporaryMailboxResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toTemporaryMailboxResponse(item))
	}

	ctx.JSON(http.StatusOK, gin.H{"items": resp})
}

/**
 * createTemporaryMailbox 为当前用户按可选租赁时间申请临时邮箱。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：已创建临时邮箱。
 * 失败条件：请求非法或域名不可用返回 400，生成或保存失败返回 500。
 */
func (server *Server) createTemporaryMailbox(ctx *gin.Context) {
	user, ok := currentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "unauthorized", "missing authenticated user")
		return
	}

	var req temporaryMailboxRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeError(ctx, http.StatusBadRequest, "invalid_request", "invalid temporary mailbox payload")
		return
	}

	result, err := server.temporary.Create(ctx.Request.Context(), user, req.Domain, req.LocalPart, req.TTLMinutes, req.Permanent)
	if errors.Is(err, service.ErrForbidden) {
		writeError(ctx, http.StatusForbidden, "forbidden", "current user cannot lease permanent mailbox")
		return
	}
	if errors.Is(err, service.ErrNoUsableDomain) {
		writeError(ctx, http.StatusBadRequest, "invalid_domain", "domain is not available for current user")
		return
	}
	if errors.Is(err, service.ErrInvalidTemporaryMailboxTTL) {
		writeError(ctx, http.StatusBadRequest, "invalid_ttl", "ttl_minutes is not allowed for current user")
		return
	}
	if errors.Is(err, service.ErrInvalidMailboxLocalPart) {
		writeError(ctx, http.StatusBadRequest, "invalid_local_part", "mailbox name must be 3-64 characters and may contain letters, numbers, dots, underscores or hyphens")
		return
	}
	if errors.Is(err, service.ErrMailboxAlreadyExists) {
		writeError(ctx, http.StatusConflict, "mailbox_exists", "mailbox address is already in use")
		return
	}
	if errors.Is(err, service.ErrDomainMailboxQuotaExceeded) {
		writeError(ctx, http.StatusConflict, "domain_mailbox_quota_exceeded", "domain mailbox quota has been reached")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "create_failed", "failed to create temporary mailbox")
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{"item": toTemporaryMailboxCreateResponse(result)})
}

/**
 * createOpenAPITemporaryMailbox 通过开放接口为 API Key 所属用户租赁临时邮箱。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：已创建临时邮箱。
 * 失败条件：认证缺失返回 401；请求非法、域名或租赁时间不可用返回 400。
 */
func (server *Server) createOpenAPITemporaryMailbox(ctx *gin.Context) {
	server.createTemporaryMailbox(ctx)
}

/**
 * getOpenAPILatestMessage 返回临时邮箱地址收到的最新一封邮件摘要。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：发件人、主题、正文和入库时间。
 * 失败条件：地址缺失返回 400；邮箱不存在、越权或没有邮件返回 404/403。
 */
func (server *Server) getOpenAPILatestMessage(ctx *gin.Context) {
	user, ok := currentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "unauthorized", "missing authenticated user")
		return
	}

	address := strings.TrimSpace(ctx.Query("address"))
	if address == "" {
		writeError(ctx, http.StatusBadRequest, "invalid_request", "address is required")
		return
	}

	message, err := server.messages.LatestForTemporaryMailbox(ctx.Request.Context(), user, address)
	if errors.Is(err, service.ErrForbidden) {
		writeError(ctx, http.StatusForbidden, "forbidden", "cannot read another user's temporary mailbox")
		return
	}
	if err != nil {
		writeNotFoundOrError(ctx, err, "message")
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"item": toOpenAPILatestMessageResponse(message)})
}

/**
 * listUsers 返回所有本地用户。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：用户 JSON 数组。
 * 失败条件：查询失败时返回 500。
 */
func (server *Server) listUsers(ctx *gin.Context) {
	users, err := server.users.ListUsers(ctx.Request.Context())
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "query_failed", "failed to list users")
		return
	}

	resp := make([]userResponse, 0, len(users))
	for _, user := range users {
		resp = append(resp, toUserResponse(user))
	}

	ctx.JSON(http.StatusOK, gin.H{"items": resp})
}

/**
 * createUser 创建本地用户账号。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：不包含密码哈希的已创建用户。
 * 失败条件：输入非法返回 400，哈希或数据库失败返回 500。
 */
func (server *Server) createUser(ctx *gin.Context) {
	var req userRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeError(ctx, http.StatusBadRequest, "invalid_request", "invalid user payload")
		return
	}

	canLeasePermanentMailbox := false
	if req.CanLeasePermanentMailbox != nil {
		canLeasePermanentMailbox = *req.CanLeasePermanentMailbox
	}

	openAIQoSRPM := 0
	if req.OpenAIQoSRPM != nil {
		openAIQoSRPM = *req.OpenAIQoSRPM
	}

	user, err := server.users.CreateUser(ctx.Request.Context(), req.Username, req.Password, req.Role, req.TemporaryMailboxTTLMinutes, canLeasePermanentMailbox, openAIQoSRPM)
	if errors.Is(err, service.ErrInvalidUser) || errors.Is(err, service.ErrInvalidTemporaryMailboxTTL) || errors.Is(err, service.ErrInvalidOpenAIQoS) {
		writeError(ctx, http.StatusBadRequest, "invalid_user", "username, password, valid role, valid temporary mailbox ttl and valid openai qos rpm are required")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "create_failed", "failed to create user")
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{"item": toUserResponse(user)})
}

/**
 * updateUser 更新已有用户的角色，并可选更新密码。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：不包含密码哈希的已更新用户。
 * 失败条件：ID 或输入非法返回 400，用户不存在返回 404，哈希失败返回 500。
 */
func (server *Server) updateUser(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}

	var req userRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeError(ctx, http.StatusBadRequest, "invalid_request", "invalid user payload")
		return
	}

	user, err := server.users.UpdateUser(ctx.Request.Context(), id, req.Username, req.Password, req.Role, req.TemporaryMailboxTTLMinutes, req.CanLeasePermanentMailbox, req.OpenAIQoSRPM)
	if errors.Is(err, service.ErrInvalidRole) {
		writeError(ctx, http.StatusBadRequest, "invalid_role", "role must be admin or user")
		return
	}
	if errors.Is(err, service.ErrInvalidTemporaryMailboxTTL) {
		writeError(ctx, http.StatusBadRequest, "invalid_ttl", "temporary mailbox ttl must be between 1 and 10080 minutes")
		return
	}
	if errors.Is(err, service.ErrInvalidOpenAIQoS) {
		writeError(ctx, http.StatusBadRequest, "invalid_openai_qos", "openai qos rpm must be between 1 and 100000")
		return
	}
	if err != nil {
		writeNotFoundOrError(ctx, err, "user")
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"item": toUserResponse(user)})
}

/**
 * deleteUser 删除用户。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：删除成功时返回 204。
 * 失败条件：ID 非法返回 400，数据库失败返回 500。
 */
func (server *Server) deleteUser(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}

	if err := server.users.DeleteUser(ctx.Request.Context(), id); err != nil {
		writeError(ctx, http.StatusInternalServerError, "delete_failed", "failed to delete user")
		return
	}

	ctx.Status(http.StatusNoContent)
}

/**
 * listDomains 返回当前用户可见的接受域名。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：域名 JSON 数组。
 * 失败条件：查询失败时返回 500。
 */
func (server *Server) listDomains(ctx *gin.Context) {
	user, _ := currentUser(ctx)
	domains, err := server.domains.ListVisible(ctx.Request.Context(), user)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "query_failed", "failed to list domains")
		return
	}

	resp := make([]domainResponse, 0, len(domains))
	for _, domain := range domains {
		resp = append(resp, toDomainResponse(domain))
	}

	ctx.JSON(http.StatusOK, gin.H{"items": resp})
}

/**
 * generateDomainVerification 为待新增域名生成 TXT 所有权验证信息。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：TXT 记录名和值。
 * 失败条件：域名非法返回 400。
 */
func (server *Server) generateDomainVerification(ctx *gin.Context) {
	user, _ := currentUser(ctx)
	var req domainVerificationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeError(ctx, http.StatusBadRequest, "invalid_request", "invalid domain verification payload")
		return
	}

	verification, err := server.domains.GenerateVerification(user, req.Domain)
	if errors.Is(err, service.ErrInvalidDomain) {
		writeError(ctx, http.StatusBadRequest, "invalid_domain", "domain must be a root domain without wildcard")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "generate_failed", "failed to generate domain verification")
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"item": toDomainVerificationResponse(verification)})
}

/**
 * createDomain 创建接受收件的域名规则。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：已创建的域名规则。
 * 失败条件：域名非法返回 400，数据库失败返回 500。
 */
func (server *Server) createDomain(ctx *gin.Context) {
	user, _ := currentUser(ctx)
	var req domainRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeError(ctx, http.StatusBadRequest, "invalid_request", "invalid domain payload")
		return
	}

	item, err := server.domains.CreateDomainWithVerification(ctx.Request.Context(), user, req.Domain, req.OwnerUserID, normalizedMailboxQuota(req.MailboxQuota), service.DomainVerificationInput{
		Name:  req.VerificationName,
		Value: req.VerificationValue,
	})
	if errors.Is(err, service.ErrInvalidDomain) {
		writeError(ctx, http.StatusBadRequest, "invalid_domain", "domain must be a root domain without wildcard")
		return
	}
	if errors.Is(err, service.ErrDomainVerification) {
		writeError(ctx, http.StatusBadRequest, "verification_failed", "domain TXT verification failed")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "create_failed", "failed to create domain")
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{"item": toDomainResponse(item)})
}

/**
 * updateDomain 更新当前用户可见的接受域名。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：已更新的域名规则。
 * 失败条件：输入非法返回 400，所有权越权返回 403，域名不存在返回 404，数据库失败返回 500。
 */
func (server *Server) updateDomain(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}

	user, _ := currentUser(ctx)
	var req domainRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeError(ctx, http.StatusBadRequest, "invalid_request", "invalid domain payload")
		return
	}

	item, err := server.domains.UpdateDomain(ctx.Request.Context(), user, id, req.Domain, req.OwnerUserID, req.Disabled, req.MailboxQuota)
	if errors.Is(err, service.ErrForbidden) {
		writeError(ctx, http.StatusForbidden, "forbidden", "cannot update another user's domain")
		return
	}
	if errors.Is(err, service.ErrInvalidDomain) {
		writeError(ctx, http.StatusBadRequest, "invalid_domain", "domain must be a root domain without wildcard")
		return
	}
	if errors.Is(err, service.ErrDomainVerification) {
		writeError(ctx, http.StatusBadRequest, "verification_failed", "domain TXT verification failed")
		return
	}
	if err != nil {
		writeNotFoundOrError(ctx, err, "domain")
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"item": toDomainResponse(item)})
}

/**
 * deleteDomain 删除当前用户可见的接受域名。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：删除成功时返回 204。
 * 失败条件：ID 非法返回 400，所有权越权返回 403，数据库失败返回 500。
 */
func (server *Server) deleteDomain(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}

	user, _ := currentUser(ctx)
	err := server.domains.DeleteDomain(ctx.Request.Context(), user, id)
	if errors.Is(err, service.ErrForbidden) {
		writeError(ctx, http.StatusForbidden, "forbidden", "cannot delete another user's domain")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "delete_failed", "failed to delete domain")
		return
	}

	ctx.Status(http.StatusNoContent)
}

/**
 * listMessages 返回当前用户可见的收件记录。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：收件记录 JSON 数组。
 * 失败条件：数据库查询失败时返回 500。
 */
func (server *Server) listMessages(ctx *gin.Context) {
	user, _ := currentUser(ctx)

	visible, err := server.messages.ListVisible(ctx.Request.Context(), user)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "query_failed", "failed to list messages")
		return
	}

	resp := make([]messageResponse, 0, len(visible))
	for _, message := range visible {
		resp = append(resp, toMessageResponse(message))
	}

	ctx.JSON(http.StatusOK, gin.H{"items": resp})
}

/**
 * getMessageBody 返回一条可见邮件的原始 DATA 和解码正文。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：请求邮件的 JSON 正文详情。
 * 失败条件：ID 非法返回 400，所有权越权返回 403，邮件不存在返回 404，数据库失败返回 500。
 */
func (server *Server) getMessageBody(ctx *gin.Context) {
	id, ok := parseID(ctx)
	if !ok {
		return
	}

	user, _ := currentUser(ctx)
	body, err := server.messages.GetBody(ctx.Request.Context(), user, id)
	if errors.Is(err, service.ErrForbidden) {
		writeError(ctx, http.StatusForbidden, "forbidden", "cannot read another user's message")
		return
	}
	if err != nil {
		writeNotFoundOrError(ctx, err, "message")
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"item": toMessageBodyResponse(body)})
}

/**
 * authMiddleware 校验 Bearer JWT 或 API Key，并加载当前数据库用户。
 *
 * 参数：无。
 * 返回值：Gin 中间件。
 * 失败条件：令牌缺失、无效或引用已删除用户时，以 401 中止请求。
 */
func (server *Server) authMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := bearerToken(ctx.GetHeader("Authorization"))
		if token == "" {
			token = strings.TrimSpace(ctx.GetHeader("X-API-Key"))
		}
		if token == "" {
			writeError(ctx, http.StatusUnauthorized, "unauthorized", "missing bearer token")
			ctx.Abort()
			return
		}

		user, err := server.users.ResolveToken(ctx.Request.Context(), token)
		if err != nil {
			writeError(ctx, http.StatusUnauthorized, "unauthorized", "invalid token")
			ctx.Abort()
			return
		}

		ctx.Set("user", user)
		ctx.Next()
	}
}

/**
 * bearerToken 从 Authorization 头中提取 token。
 *
 * 参数：
 * - header：Authorization 头字段值。
 * 返回值：头字段使用 Bearer scheme 时返回 token。
 * 失败条件：无。
 */
func bearerToken(header string) string {
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

/**
 * requireAdmin 阻止非管理员访问仅管理员可用的路由。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：无。
 * 失败条件：当前用户不是管理员时，以 403 中止请求。
 */
func requireAdmin(ctx *gin.Context) {
	user, ok := currentUser(ctx)
	if !ok || user.Role != storage.RoleAdmin {
		writeError(ctx, http.StatusForbidden, "forbidden", "admin role required")
		ctx.Abort()
		return
	}

	ctx.Next()
}

/**
 * currentUser 从 Gin 上下文中取出认证用户。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：中间件已写入用户时返回用户和 true。
 * 失败条件：无。
 */
func currentUser(ctx *gin.Context) (storage.User, bool) {
	value, ok := ctx.Get("user")
	if !ok {
		return storage.User{}, false
	}

	user, ok := value.(storage.User)
	return user, ok
}

/**
 * parseID 解析路由 id 参数。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * 返回值：ID 有效时返回解析后的 uint ID 和 true。
 * 失败条件：ID 非法时写入 400 并返回 false。
 */
func parseID(ctx *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		writeError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return 0, false
	}

	return uint(id), true
}

/**
 * toUserResponse 从 API 响应中隐藏用户敏感字段。
 *
 * 参数：
 * - user：存储层用户模型。
 * 返回值：公开用户 DTO。
 * 失败条件：无。
 */
func toUserResponse(user storage.User) userResponse {
	dto := service.ToUserDTO(user)
	return userResponse{
		ID:                         dto.ID,
		Username:                   dto.Username,
		Role:                       dto.Role,
		HasAPIKey:                  dto.HasAPIKey,
		TemporaryMailboxTTLMinutes: dto.TemporaryMailboxTTLMinutes,
		CanLeasePermanentMailbox:   dto.CanLeasePermanentMailbox,
		OpenAIQoSRPM:               dto.OpenAIQoSRPM,
		CreatedAt:                  dto.CreatedAt,
		UpdatedAt:                  dto.UpdatedAt,
	}
}

/**
 * toDomainResponse 将域名模型转换为 API DTO。
 *
 * 参数：
 * - domain：存储层域名模型。
 * 返回值：公开域名 DTO。
 * 失败条件：无。
 */
func toDomainResponse(domain storage.AcceptedDomain) domainResponse {
	ownerName := "所有人"
	if domain.Owner != nil {
		ownerName = domain.Owner.Username
	}

	return domainResponse{
		ID:           domain.ID,
		Domain:       domain.Domain,
		OwnerUserID:  domain.OwnerUserID,
		OwnerName:    ownerName,
		Disabled:     domain.Disabled,
		MailboxQuota: domain.MailboxQuota,
		CreatedAt:    domain.CreatedAt,
		UpdatedAt:    domain.UpdatedAt,
	}
}

/**
 * normalizedMailboxQuota 将空值额度归一为不限额。
 *
 * 参数：
 * - value：请求体里的可选邮箱额度。
 * 返回值：nil 或 0 表示不限额；正数表示累计创建邮箱数量上限；负数原样保留给 service 校验。
 * 失败条件：无。
 */
func normalizedMailboxQuota(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

/**
 * toDomainVerificationResponse 将 service 验证信息转换为 API DTO。
 *
 * 参数：
 * - verification：service 生成的 TXT 验证信息。
 * 返回值：公开给前端展示和提交的验证 DTO。
 * 失败条件：无。
 */
func toDomainVerificationResponse(verification service.DomainVerification) domainVerificationResponse {
	return domainVerificationResponse{
		Name:  verification.Name,
		Value: verification.Value,
	}
}

/**
 * toMessageResponse 将已存储邮件转换为 API DTO。
 *
 * 参数：
 * - message：存储层邮件模型。
 * 返回值：不包含正文载荷的公开邮件列表 DTO。
 * 失败条件：无。
 */
func toMessageResponse(message storage.Message) messageResponse {
	return messageResponse{
		ID:         message.ID,
		HeloName:   message.HeloName,
		MailFrom:   message.MailFrom,
		RcptTo:     message.RcptTo,
		RemoteAddr: message.RemoteAddr,
		CreatedAt:  message.CreatedAt,
	}
}

/**
 * toMessageBodyResponse 将 service 正文结果转换为 API DTO。
 *
 * 参数：
 * - body：service 返回的正文详情。
 * 返回值：包含原始 DATA 和解码正文的正文详情 DTO。
 * 失败条件：无。
 */
func toMessageBodyResponse(body service.MessageBody) messageBodyResponse {
	return messageBodyResponse{
		ID:     body.Message.ID,
		Data:   body.Message.Data,
		Body:   body.Decoded.Body,
		HTML:   body.Decoded.HTML,
		IsHTML: body.Decoded.IsHTML,
	}
}

/**
 * toTemporaryMailboxResponse 将临时邮箱模型转换为 API DTO。
 *
 * 参数：
 * - mailbox：临时邮箱模型。
 * 返回值：临时邮箱响应 DTO。
 * 失败条件：无。
 */
func toTemporaryMailboxResponse(mailbox storage.TemporaryMailbox) temporaryMailboxResponse {
	return temporaryMailboxResponse{
		ID:          mailbox.ID,
		Address:     mailbox.Address,
		LocalPart:   mailbox.LocalPart,
		Domain:      mailbox.Domain,
		OwnerUserID: mailbox.OwnerUserID,
		ExpiresAt:   mailbox.ExpiresAt,
		IsPermanent: mailbox.IsPermanent,
		CreatedAt:   mailbox.CreatedAt,
		Expired:     !mailbox.IsPermanent && time.Now().After(mailbox.ExpiresAt),
	}
}

/**
 * toTemporaryMailboxCreateResponse 将临时邮箱创建结果转换为 API DTO。
 *
 * 参数：
 * - result：service 返回的临时邮箱创建结果。
 * 返回值：带有效分钟数的临时邮箱响应 DTO。
 * 失败条件：无。
 */
func toTemporaryMailboxCreateResponse(result service.TemporaryMailboxResult) temporaryMailboxCreateResponse {
	return temporaryMailboxCreateResponse{
		temporaryMailboxResponse: toTemporaryMailboxResponse(result.Mailbox),
		TTLMinutes:               result.TTLMinutes,
	}
}

/**
 * toOpenAPILatestMessageResponse 将 service 最新邮件摘要转换为开放接口响应。
 *
 * 参数：
 * - message：service 返回的最新邮件摘要。
 * 返回值：开放接口最新邮件 DTO。
 * 失败条件：无。
 */
func toOpenAPILatestMessageResponse(message service.OpenAPILatestMessage) openAPILatestMessageResponse {
	return openAPILatestMessageResponse{
		From:      message.From,
		Subject:   message.Subject,
		Body:      message.Body,
		CreatedAt: message.CreatedAt,
	}
}

/**
 * writeNotFoundOrError 将 GORM 查询失败映射为稳定的 API 响应。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * - err：数据库错误。
 * - resource：用于用户可读错误消息的资源名称。
 * 返回值：无。
 * 失败条件：无。
 */
func writeNotFoundOrError(ctx *gin.Context, err error, resource string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "not_found", resource+" not found")
		return
	}

	writeError(ctx, http.StatusInternalServerError, "query_failed", "failed to load "+resource)
}

/**
 * writeError 发送稳定结构的 JSON 错误响应。
 *
 * 参数：
 * - ctx：Gin 请求上下文。
 * - status：HTTP 状态码。
 * - code：机器可读错误码。
 * - message：人类可读错误消息。
 * 返回值：无。
 * 失败条件：无。
 */
func writeError(ctx *gin.Context, status int, code string, message string) {
	ctx.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
