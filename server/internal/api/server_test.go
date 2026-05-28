package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"mx-mail-api/internal/config"
	"mx-mail-api/internal/repository"
	"mx-mail-api/internal/service"
	"mx-mail-api/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

/**
 * TestLoginAndDomainOwnership 校验 JWT 登录和域名可见性规则。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：初始化、登录、域名所有权或角色授权行为退化时测试失败。
 */
func TestLoginAndDomainOwnership(t *testing.T) {
	router, db := newTestRouter(t)

	adminToken := loginAndToken(t, router, "admin", "admin123456")
	userID := createUser(t, router, adminToken, "alice", "password123", storage.RoleUser)
	userToken := loginAndToken(t, router, "alice", "password123")

	createDomain(t, router, userToken, "example.test", nil, http.StatusCreated)
	createDomain(t, router, adminToken, "*.admin.test", uintPtr(1), http.StatusBadRequest)
	createDomain(t, router, adminToken, "admin.test", uintPtr(1), http.StatusCreated)
	createDomain(t, router, userToken, "other.test", uintPtr(userID), http.StatusCreated)

	userDomains := listDomains(t, router, userToken)
	if len(userDomains) != 2 {
		t.Fatalf("expected ordinary user to see 2 owned domains, got %#v", userDomains)
	}

	adminDomains := listDomains(t, router, adminToken)
	if len(adminDomains) != 3 {
		t.Fatalf("expected admin to see all 3 domains, got %#v", adminDomains)
	}

	var domains []storage.AcceptedDomain
	if err := db.Find(&domains).Error; err != nil {
		t.Fatalf("failed to query domains: %v", err)
	}
}

/**
 * TestListMessagesUsesDomainOwnership 校验所有用户仅能查看发往自己域名的收件记录。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：收件记录可见性跨域名所有权边界泄漏时测试失败。
 */
func TestListMessagesUsesDomainOwnership(t *testing.T) {
	router, db := newTestRouter(t)

	adminToken := loginAndToken(t, router, "admin", "admin123456")
	userID := createUser(t, router, adminToken, "alice", "password123", storage.RoleUser)
	userToken := loginAndToken(t, router, "alice", "password123")
	createDomain(t, router, userToken, "example.test", nil, http.StatusCreated)
	createDomain(t, router, adminToken, "admin.test", uintPtr(1), http.StatusCreated)
	createDomain(t, router, adminToken, "global.test", nil, http.StatusCreated)

	if err := db.Create(&[]storage.Message{
		{
			HeloName:   "sender.test",
			MailFrom:   "sender@remote.test",
			RcptTo:     []string{"inbox@example.test"},
			Data:       "Subject: owned\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nbody=20text=0A",
			RemoteAddr: "127.0.0.1:12345",
		},
		{
			HeloName:   "sender.test",
			MailFrom:   "sender@remote.test",
			RcptTo:     []string{"inbox@admin.test"},
			Data:       "Subject: hidden\n\nbody\n",
			RemoteAddr: "127.0.0.1:12346",
		},
		{
			HeloName:   "sender.test",
			MailFrom:   "sender@remote.test",
			RcptTo:     []string{"inbox@global.test"},
			Data:       "Subject: global\n\nbody\n",
			RemoteAddr: "127.0.0.1:12347",
		},
	}).Error; err != nil {
		t.Fatalf("failed to seed messages: %v", err)
	}

	userMessages := listMessages(t, router, userToken)
	if len(userMessages) != 2 {
		t.Fatalf("expected ordinary user to see owned and global messages, got %#v for user %d", userMessages, userID)
	}
	if !containsRecipient(userMessages, "inbox@example.test") || !containsRecipient(userMessages, "inbox@global.test") {
		t.Fatalf("expected ordinary user messages to include owned and global recipients, got %#v", userMessages)
	}

	adminMessages := listMessages(t, router, adminToken)
	if len(adminMessages) != 2 {
		t.Fatalf("expected admin to see admin-owned and global messages, got %#v", adminMessages)
	}
	if !containsRecipient(adminMessages, "inbox@admin.test") || !containsRecipient(adminMessages, "inbox@global.test") {
		t.Fatalf("expected admin messages to include owned and global recipients, got %#v", adminMessages)
	}

	ownedMessage := findMessageByRecipient(t, userMessages, "inbox@example.test")
	globalMessage := findMessageByRecipient(t, userMessages, "inbox@global.test")

	body := getMessageBody(t, router, userToken, ownedMessage.ID, http.StatusOK)
	if body.Data == body.Body {
		t.Fatalf("expected raw DATA and decoded body to be different, got data=%q body=%q", body.Data, body.Body)
	}
	if body.Body != "body text\n" {
		t.Fatalf("expected decoded message body, got %q", body.Body)
	}
	getMessageBody(t, router, userToken, globalMessage.ID, http.StatusOK)

	adminOwnedMessage := findMessageByRecipient(t, adminMessages, "inbox@admin.test")
	adminGlobalMessage := findMessageByRecipient(t, adminMessages, "inbox@global.test")
	getMessageBody(t, router, adminToken, adminOwnedMessage.ID, http.StatusOK)
	getMessageBody(t, router, adminToken, adminGlobalMessage.ID, http.StatusOK)
	getMessageBody(t, router, userToken, adminOwnedMessage.ID, http.StatusForbidden)
	getMessageBody(t, router, adminToken, ownedMessage.ID, http.StatusForbidden)
}

/**
 * TestAPIKeyAuthenticatesAsCurrentUser 校验生成的 API Key 拥有与 JWT 相同的访问规则。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：API Key 生成、一次性返回或认证行为退化时测试失败。
 */
func TestAPIKeyAuthenticatesAsCurrentUser(t *testing.T) {
	router, _ := newTestRouter(t)

	adminToken := loginAndToken(t, router, "admin", "admin123456")
	createDomain(t, router, adminToken, "admin.test", uintPtr(1), http.StatusCreated)

	apiKey := resetAPIKey(t, router, adminToken)
	if apiKey == "" {
		t.Fatal("expected plaintext api key")
	}

	resp := performJSON(t, router, http.MethodGet, "/api/domains", apiKey, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected api key to authenticate, got %d body %s", resp.Code, resp.Body.String())
	}
}

/**
 * TestTemporaryMailboxCreateAndList 校验用户可以基于自己可用域名申请默认 30 分钟临时邮箱。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：临时邮箱创建、有效期或列表返回行为退化时测试失败。
 */
func TestTemporaryMailboxCreateAndList(t *testing.T) {
	router, db := newTestRouter(t)

	adminToken := loginAndToken(t, router, "admin", "admin123456")
	userID := createUser(t, router, adminToken, "alice", "password123", storage.RoleUser)
	userToken := loginAndToken(t, router, "alice", "password123")
	createDomain(t, router, userToken, "example.test", uintPtr(userID), http.StatusCreated)

	resp := performJSON(t, router, http.MethodPost, "/api/temporary-mailboxes", userToken, temporaryMailboxRequest{
		Domain: "example.test",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected temporary mailbox status 201, got %d body %s", resp.Code, resp.Body.String())
	}

	var created struct {
		Item temporaryMailboxCreateResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse temporary mailbox response: %v", err)
	}
	if created.Item.TTLMinutes != 30 {
		t.Fatalf("expected ttl 30 minutes, got %d", created.Item.TTLMinutes)
	}
	if created.Item.Domain != "example.test" || created.Item.Address == "" || created.Item.Expired {
		t.Fatalf("unexpected temporary mailbox response: %#v", created.Item)
	}

	if err := db.Create(&storage.TemporaryMailbox{
		Address:     "expired@example.test",
		LocalPart:   "expired",
		Domain:      "example.test",
		OwnerUserID: userID,
		ExpiresAt:   created.Item.CreatedAt.Add(-time.Minute),
	}).Error; err != nil {
		t.Fatalf("failed to seed expired temporary mailbox: %v", err)
	}

	resp = performJSON(t, router, http.MethodGet, "/api/temporary-mailboxes", userToken, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected list temporary mailboxes status 200, got %d body %s", resp.Code, resp.Body.String())
	}
	var listed struct {
		Items []temporaryMailboxResponse `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &listed); err != nil {
		t.Fatalf("failed to parse temporary mailbox list response: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].Address != created.Item.Address {
		t.Fatalf("expected only active temporary mailbox in list, got %#v", listed.Items)
	}
}

/**
 * TestTemporaryMailboxUsesConfiguredTTL 校验管理员配置的用户租赁时长会限制临时邮箱申请。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：用户可选租赁时间未生效、非法时间未拒绝，或旧客户端默认值不兼容时测试失败。
 */
func TestTemporaryMailboxUsesConfiguredTTL(t *testing.T) {
	router, _ := newTestRouter(t)

	adminToken := loginAndToken(t, router, "admin", "admin123456")
	userID := createUser(t, router, adminToken, "alice", "password123", storage.RoleUser)
	updateUserTTL(t, router, adminToken, userID, []int{60, 15, 15})
	userToken := loginAndToken(t, router, "alice", "password123")
	createDomain(t, router, userToken, "ttl.test", uintPtr(userID), http.StatusCreated)

	ttl := 60
	resp := performJSON(t, router, http.MethodPost, "/api/temporary-mailboxes", userToken, temporaryMailboxRequest{
		Domain:     "ttl.test",
		TTLMinutes: &ttl,
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected configured ttl status 201, got %d body %s", resp.Code, resp.Body.String())
	}

	var created struct {
		Item temporaryMailboxCreateResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse configured ttl response: %v", err)
	}
	if created.Item.TTLMinutes != 60 {
		t.Fatalf("expected ttl 60 minutes, got %d", created.Item.TTLMinutes)
	}

	invalidTTL := 30
	resp = performJSON(t, router, http.MethodPost, "/api/temporary-mailboxes", userToken, temporaryMailboxRequest{
		Domain:     "ttl.test",
		TTLMinutes: &invalidTTL,
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid ttl status 400, got %d body %s", resp.Code, resp.Body.String())
	}

	resp = performJSON(t, router, http.MethodPost, "/api/temporary-mailboxes", userToken, temporaryMailboxRequest{
		Domain: "ttl.test",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected default configured ttl status 201, got %d body %s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse default configured ttl response: %v", err)
	}
	if created.Item.TTLMinutes != 15 {
		t.Fatalf("expected omitted ttl to use first configured ttl 15 minutes, got %d", created.Item.TTLMinutes)
	}
}

/**
 * TestTemporaryMailboxRandomDomain 校验未提交域名时会从当前用户可用域名中随机选择。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：空域名申请失败，或旧 "*" 通配配置仍可创建时测试失败。
 */
func TestTemporaryMailboxRandomDomain(t *testing.T) {
	router, _ := newTestRouter(t)

	adminToken := loginAndToken(t, router, "admin", "admin123456")
	userID := createUser(t, router, adminToken, "alice", "password123", storage.RoleUser)
	userToken := loginAndToken(t, router, "alice", "password123")
	createDomain(t, router, userToken, "*.wild.test", uintPtr(userID), http.StatusBadRequest)
	createDomain(t, router, userToken, "random.test", uintPtr(userID), http.StatusCreated)

	resp := performJSON(t, router, http.MethodPost, "/api/temporary-mailboxes", userToken, temporaryMailboxRequest{})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected random domain status 201, got %d body %s", resp.Code, resp.Body.String())
	}

	var created struct {
		Item temporaryMailboxCreateResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse random domain response: %v", err)
	}
	if created.Item.Domain != "random.test" {
		t.Fatalf("expected random domain to use exact domain random.test, got %#v", created.Item)
	}
	if strings.HasPrefix(created.Item.LocalPart, "tmp-") || strings.HasPrefix(created.Item.Address, "tmp-") {
		t.Fatalf("expected random mailbox without tmp- prefix, got %#v", created.Item)
	}
	if len(created.Item.LocalPart) != 8 {
		t.Fatalf("expected random local part length 8, got %q", created.Item.LocalPart)
	}

}

/**
 * TestDisabledDomainCannotLeaseOrReceive 校验禁用域名不再用于邮箱申请和 SMTP 收件。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：禁用域名仍能申请邮箱、随机选择或通过收件域名匹配时测试失败。
 */
func TestDisabledDomainCannotLeaseOrReceive(t *testing.T) {
	router, db := newTestRouter(t)

	adminToken := loginAndToken(t, router, "admin", "admin123456")
	userID := createUser(t, router, adminToken, "alice", "password123", storage.RoleUser)
	userToken := loginAndToken(t, router, "alice", "password123")
	createDomain(t, router, userToken, "disabled.test", uintPtr(userID), http.StatusCreated)
	createDomain(t, router, userToken, "enabled.test", uintPtr(userID), http.StatusCreated)

	var disabledDomain storage.AcceptedDomain
	if err := db.Where("domain = ?", "disabled.test").First(&disabledDomain).Error; err != nil {
		t.Fatalf("failed to load disabled domain: %v", err)
	}
	disabled := true
	resp := performJSON(t, router, http.MethodPut, "/api/domains/"+strconv.FormatUint(uint64(disabledDomain.ID), 10), userToken, domainRequest{
		Domain:   "disabled.test",
		Disabled: &disabled,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected disable domain status 200, got %d body %s", resp.Code, resp.Body.String())
	}

	resp = performJSON(t, router, http.MethodPost, "/api/temporary-mailboxes", userToken, temporaryMailboxRequest{
		Domain: "disabled.test",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected disabled domain lease status 400, got %d body %s", resp.Code, resp.Body.String())
	}

	resp = performJSON(t, router, http.MethodPost, "/api/temporary-mailboxes", userToken, temporaryMailboxRequest{})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected random lease to use enabled domain, got %d body %s", resp.Code, resp.Body.String())
	}
	var created struct {
		Item temporaryMailboxCreateResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse random lease response: %v", err)
	}
	if created.Item.Domain != "enabled.test" {
		t.Fatalf("expected random lease to skip disabled domain, got %#v", created.Item)
	}

	patterns, err := repository.NewDomainRepository(db).AcceptedPatterns(context.Background())
	if err != nil {
		t.Fatalf("failed to load accepted patterns: %v", err)
	}
	if len(patterns) != 1 || patterns[0] != "enabled.test" {
		t.Fatalf("expected only enabled domain in SMTP patterns, got %#v", patterns)
	}
}

/**
 * newTestRouter 创建由内存 SQLite 支撑的隔离 API 路由。
 *
 * 参数：
 * - t：测试辅助对象。
 * 返回值：Gin 引擎和数据库句柄。
 * 失败条件：schema 迁移或初始化失败时测试失败。
 */
func newTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&storage.User{}, &storage.AcceptedDomain{}, &storage.TemporaryMailbox{}, &storage.Message{}); err != nil {
		t.Fatalf("failed to migrate sqlite schema: %v", err)
	}

	userRepo := repository.NewUserRepository(db)
	domainRepo := repository.NewDomainRepository(db)
	messageRepo := repository.NewMessageRepository(db)
	temporaryMailboxRepo := repository.NewTemporaryMailboxRepository(db)
	temporaryMailboxService := service.NewTemporaryMailboxService(temporaryMailboxRepo, domainRepo)
	userService := service.NewUserService(userRepo, config.Config{
		Auth: config.AuthConfig{
			JWTSecret:     "test-secret",
			TokenTTLHours: 24,
		},
		Admin: config.AdminConfig{
			InitialUser: config.InitialAdminUser{
				Username: "admin",
				Password: "admin123456",
			},
		},
	})
	if err := userService.BootstrapInitialAdmin(context.Background()); err != nil {
		t.Fatalf("failed to bootstrap admin: %v", err)
	}

	testConfig := config.Config{
		SMTP: config.SMTPConfig{
			Addr:     ":12525",
			Hostname: "mail.test",
		},
	}
	domainService := service.NewDomainService(domainRepo, testConfig)
	domainService.SetTXTLookupForTest(func(_ context.Context, name string) ([]string, error) {
		return []string{
			"eeb66cbc8c1815588baafa500db01084",
			"735d9545dd0be330ab7d9550c92a9768",
		}, nil
	})

	server := New(
		userService,
		domainService,
		service.NewMessageService(messageRepo, domainRepo, temporaryMailboxService),
		temporaryMailboxService,
		testConfig,
	)
	router := gin.New()
	server.RegisterRoutes(router)
	return router, db
}

/**
 * loginAndToken 登录并提取 JWT token。
 *
 * 参数：
 * - t：测试辅助对象。
 * - router：API 路由。
 * - username：登录用户名。
 * - password：登录密码。
 * 返回值：JWT 访问令牌。
 * 失败条件：登录失败或 token 缺失时测试失败。
 */
func loginAndToken(t *testing.T, router *gin.Engine, username string, password string) string {
	t.Helper()

	resp := performJSON(t, router, http.MethodPost, "/api/auth/login", "", map[string]string{
		"username": username,
		"password": password,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected login status 200, got %d body %s", resp.Code, resp.Body.String())
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse login response: %v", err)
	}
	if body.Token == "" {
		t.Fatal("expected login token")
	}

	return body.Token
}

/**
 * createUser 通过管理员 API 创建用户。
 *
 * 参数：
 * - t：测试辅助对象。
 * - router：API 路由。
 * - token：管理员 JWT token。
 * - username：新用户名。
 * - password：新密码。
 * - role：新用户角色。
 * 返回值：已创建用户 ID。
 * 失败条件：API 未返回 201 时测试失败。
 */
func createUser(t *testing.T, router *gin.Engine, token string, username string, password string, role string) uint {
	t.Helper()

	resp := performJSON(t, router, http.MethodPost, "/api/users", token, map[string]string{
		"username": username,
		"password": password,
		"role":     role,
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected create user status 201, got %d body %s", resp.Code, resp.Body.String())
	}

	var body struct {
		Item userResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse create user response: %v", err)
	}

	return body.Item.ID
}

/**
 * updateUserTTL 通过管理员 API 更新用户临时邮箱可选租赁时间。
 *
 * 参数：
 * - t：测试辅助对象。
 * - router：API 路由。
 * - token：管理员 JWT token。
 * - userID：目标用户 ID。
 * - values：租赁分钟数列表。
 * 返回值：无。
 * 失败条件：API 未返回 200，或响应未按升序去重保存时测试失败。
 */
func updateUserTTL(t *testing.T, router *gin.Engine, token string, userID uint, values []int) {
	t.Helper()

	resp := performJSON(t, router, http.MethodPut, "/api/users/"+strconv.FormatUint(uint64(userID), 10), token, userRequest{
		TemporaryMailboxTTLMinutes: values,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected update user ttl status 200, got %d body %s", resp.Code, resp.Body.String())
	}

	var body struct {
		Item userResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse update user ttl response: %v", err)
	}
	if got := body.Item.TemporaryMailboxTTLMinutes; len(got) != 2 || got[0] != 15 || got[1] != 60 {
		t.Fatalf("expected ttl values to be normalized to [15 60], got %#v", got)
	}
}

/**
 * TestOpenAPITemporaryMailboxLeaseAndLatestMessage 校验开放接口可租赁临时邮箱并读取该地址最新邮件。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：API Key 认证、租赁归属、最新邮件筛选或正文解析退化时测试失败。
 */
func TestOpenAPITemporaryMailboxLeaseAndLatestMessage(t *testing.T) {
	router, db := newTestRouter(t)

	adminToken := loginAndToken(t, router, "admin", "admin123456")
	userID := createUser(t, router, adminToken, "alice", "password123", storage.RoleUser)
	userToken := loginAndToken(t, router, "alice", "password123")
	createDomain(t, router, userToken, "open.test", uintPtr(userID), http.StatusCreated)
	apiKey := resetAPIKey(t, router, userToken)

	resp := performJSON(t, router, http.MethodPost, "/openapi/temporary-mailboxes", apiKey, temporaryMailboxRequest{
		Domain: "open.test",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected openapi lease status 201, got %d body %s", resp.Code, resp.Body.String())
	}

	var lease struct {
		Item temporaryMailboxCreateResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &lease); err != nil {
		t.Fatalf("failed to parse openapi lease response: %v", err)
	}
	if lease.Item.Address == "" || lease.Item.Domain != "open.test" {
		t.Fatalf("unexpected openapi lease response: %#v", lease.Item)
	}

	if err := db.Create(&[]storage.Message{
		{
			HeloName:   "sender.test",
			MailFrom:   "old@remote.test",
			RcptTo:     []string{lease.Item.Address},
			Data:       "From: old@remote.test\r\nSubject: old\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nold body",
			RemoteAddr: "127.0.0.1:10001",
		},
		{
			HeloName:   "sender.test",
			MailFrom:   "new@remote.test",
			RcptTo:     []string{lease.Item.Address},
			Data:       "From: new@remote.test\r\nSubject: =?UTF-8?Q?=E6=96=B0=E9=82=AE=E4=BB=B6?=\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nnew body",
			RemoteAddr: "127.0.0.1:10002",
		},
	}).Error; err != nil {
		t.Fatalf("failed to seed openapi messages: %v", err)
	}

	resp = performJSON(t, router, http.MethodGet, "/openapi/temporary-mailboxes/latest-message?address="+lease.Item.Address, apiKey, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected latest message status 200, got %d body %s", resp.Code, resp.Body.String())
	}

	var latest struct {
		Item openAPILatestMessageResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &latest); err != nil {
		t.Fatalf("failed to parse openapi latest message response: %v", err)
	}
	if latest.Item.From != "new@remote.test" || latest.Item.Subject != "新邮件" || latest.Item.Body != "new body" {
		t.Fatalf("unexpected latest message: %#v", latest.Item)
	}

	resp = performJSON(t, router, http.MethodGet, "/openapi/temporary-mailboxes/latest-message?address=missing@open.test", apiKey, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected missing mailbox status 404, got %d body %s", resp.Code, resp.Body.String())
	}
}

/**
 * TestCreateDomainRequiresTXTVerification 校验新增域名必须通过 TXT 所有权验证。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：缺少 TXT 校验仍能新增，或生成验证接口不返回记录名和值时测试失败。
 */
func TestCreateDomainRequiresTXTVerification(t *testing.T) {
	router, _ := newTestRouter(t)

	adminToken := loginAndToken(t, router, "admin", "admin123456")
	userID := createUser(t, router, adminToken, "alice", "password123", storage.RoleUser)
	userToken := loginAndToken(t, router, "alice", "password123")

	resp := performJSON(t, router, http.MethodPost, "/api/domains/verification", userToken, domainVerificationRequest{
		Domain: "verify.test",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected verification status 200, got %d body %s", resp.Code, resp.Body.String())
	}
	var verification struct {
		Item domainVerificationResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &verification); err != nil {
		t.Fatalf("failed to parse verification response: %v", err)
	}
	if !isAlnumRecordForDomain(verification.Item.Name, "verify.test") || verification.Item.Value != "eeb66cbc8c1815588baafa500db01084" {
		t.Fatalf("unexpected verification response: %#v", verification.Item)
	}

	resp = performJSON(t, router, http.MethodPost, "/api/domains/verification", userToken, domainVerificationRequest{})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected verification without domain status 200, got %d body %s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &verification); err != nil {
		t.Fatalf("failed to parse verification without domain response: %v", err)
	}
	if !isAlnumLabel(verification.Item.Name) || verification.Item.Value != "eeb66cbc8c1815588baafa500db01084" {
		t.Fatalf("unexpected verification without domain response: %#v", verification.Item)
	}

	resp = performJSON(t, router, http.MethodPost, "/api/domains", userToken, domainRequest{
		Domain:      "verify.test",
		OwnerUserID: uintPtr(userID),
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected missing verification status 400, got %d body %s", resp.Code, resp.Body.String())
	}
}

/**
 * createDomain 创建域名并检查预期状态码。
 *
 * 参数：
 * - t：测试辅助对象。
 * - router：API 路由。
 * - token：JWT token。
 * - domain：域名规则。
 * - ownerID：可选所有者 ID。
 * - expectedStatus：预期 HTTP 状态码。
 * 返回值：无。
 * 失败条件：状态码不一致时测试失败。
 */
func createDomain(t *testing.T, router *gin.Engine, token string, domain string, ownerID *uint, expectedStatus int) {
	t.Helper()

	verificationName := ""
	verificationValue := ""
	if !strings.Contains(domain, "*") {
		verifyResp := performJSON(t, router, http.MethodPost, "/api/domains/verification", token, domainVerificationRequest{
			Domain: domain,
		})
		if verifyResp.Code != http.StatusOK {
			t.Fatalf("expected create domain verification status 200, got %d body %s", verifyResp.Code, verifyResp.Body.String())
		}
		var verifyBody struct {
			Item domainVerificationResponse `json:"item"`
		}
		if err := json.Unmarshal(verifyResp.Body.Bytes(), &verifyBody); err != nil {
			t.Fatalf("failed to parse create domain verification response: %v", err)
		}
		verificationName = verifyBody.Item.Name
		verificationValue = verifyBody.Item.Value
	}

	resp := performJSON(t, router, http.MethodPost, "/api/domains", token, domainRequest{
		Domain:            domain,
		OwnerUserID:       ownerID,
		VerificationName:  verificationName,
		VerificationValue: verificationValue,
	})
	if resp.Code != expectedStatus {
		t.Fatalf("expected create domain status %d, got %d body %s", expectedStatus, resp.Code, resp.Body.String())
	}
}

/**
 * isAlnumRecordForDomain 检查 TXT 验证记录名是否为随机字母数字前缀加指定域名。
 *
 * 参数：
 * - name：接口返回的完整 TXT 记录名。
 * - domain：接口请求中的域名。
 * 返回值：记录名前缀仅包含字母数字且域名后缀一致时返回 true。
 * 失败条件：无。
 */
func isAlnumRecordForDomain(name string, domain string) bool {
	suffix := "." + domain
	if !strings.HasSuffix(name, suffix) {
		return false
	}

	return isAlnumLabel(strings.TrimSuffix(name, suffix))
}

/**
 * isAlnumLabel 检查字符串是否仅包含小写字母和数字。
 *
 * 参数：
 * - value：待检查字符串。
 * 返回值：仅包含小写字母和数字时返回 true。
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
 * containsRecipient 检查邮件列表是否包含指定收件地址。
 *
 * 参数：
 * - messages：邮件 DTO 列表。
 * - recipient：目标收件地址。
 * 返回值：任一邮件包含该收件人时返回 true。
 * 失败条件：无。
 */
func containsRecipient(messages []messageResponse, recipient string) bool {
	for _, message := range messages {
		for _, value := range message.RcptTo {
			if value == recipient {
				return true
			}
		}
	}

	return false
}

/**
 * findMessageByRecipient 返回包含目标收件人的邮件。
 *
 * 参数：
 * - t：测试辅助对象。
 * - messages：邮件 DTO 列表。
 * - recipient：目标收件地址。
 * 返回值：匹配的邮件 DTO。
 * 失败条件：没有匹配邮件时测试失败。
 */
func findMessageByRecipient(t *testing.T, messages []messageResponse, recipient string) messageResponse {
	t.Helper()

	for _, message := range messages {
		for _, value := range message.RcptTo {
			if value == recipient {
				return message
			}
		}
	}

	t.Fatalf("expected message with recipient %s in %#v", recipient, messages)
	return messageResponse{}
}

/**
 * uintPtr 为 API 测试中的可选所有者 ID 返回指针。
 *
 * 参数：
 * - value：所有者 ID。
 * 返回值：指向 value 的指针。
 * 失败条件：无。
 */
func uintPtr(value uint) *uint {
	return &value
}

/**
 * listDomains 列出认证用户可见的域名。
 *
 * 参数：
 * - t：测试辅助对象。
 * - router：API 路由。
 * - token：JWT token。
 * 返回值：可见域名 DTO 列表。
 * 失败条件：API 未返回 200 时测试失败。
 */
func listDomains(t *testing.T, router *gin.Engine, token string) []domainResponse {
	t.Helper()

	resp := performJSON(t, router, http.MethodGet, "/api/domains", token, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected list domains status 200, got %d body %s", resp.Code, resp.Body.String())
	}

	var body struct {
		Items []domainResponse `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse domains response: %v", err)
	}

	return body.Items
}

/**
 * listMessages 列出认证用户可见的收件记录。
 *
 * 参数：
 * - t：测试辅助对象。
 * - router：API 路由。
 * - token：JWT token。
 * 返回值：可见邮件 DTO 列表。
 * 失败条件：API 未返回 200 时测试失败。
 */
func listMessages(t *testing.T, router *gin.Engine, token string) []messageResponse {
	t.Helper()

	resp := performJSON(t, router, http.MethodGet, "/api/messages", token, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected list messages status 200, got %d body %s", resp.Code, resp.Body.String())
	}

	var body struct {
		Items []messageResponse `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse messages response: %v", err)
	}

	return body.Items
}

/**
 * getMessageBody 获取一封邮件的正文详情。
 *
 * 参数：
 * - t：测试辅助对象。
 * - router：API 路由。
 * - token：JWT token。
 * - id：邮件 ID。
 * - expectedStatus：预期 HTTP 状态码。
 * 返回值：请求成功时返回正文详情。
 * 失败条件：API 状态码不一致，或成功响应无法解析时测试失败。
 */
func getMessageBody(t *testing.T, router *gin.Engine, token string, id uint, expectedStatus int) messageBodyResponse {
	t.Helper()

	resp := performJSON(t, router, http.MethodGet, "/api/messages/"+strconv.FormatUint(uint64(id), 10)+"/body", token, nil)
	if resp.Code != expectedStatus {
		t.Fatalf("expected get message body status %d, got %d body %s", expectedStatus, resp.Code, resp.Body.String())
	}
	if expectedStatus != http.StatusOK {
		return messageBodyResponse{}
	}

	var body struct {
		Item messageBodyResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse message body response: %v", err)
	}

	return body.Item
}

/**
 * resetAPIKey 通过当前用户接口生成 API Key。
 *
 * 参数：
 * - t：测试辅助对象。
 * - router：API 路由。
 * - token：JWT token。
 * 返回值：明文 API Key。
 * 失败条件：接口未返回 token 时测试失败。
 */
func resetAPIKey(t *testing.T, router *gin.Engine, token string) string {
	t.Helper()

	resp := performJSON(t, router, http.MethodPost, "/api/me/api-key", token, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected reset api key status 200, got %d body %s", resp.Code, resp.Body.String())
	}

	var body struct {
		Item apiKeyResponse `json:"item"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse api key response: %v", err)
	}

	return body.Item.Token
}

/**
 * performJSON 向测试路由发送 JSON 请求。
 *
 * 参数：
 * - t：测试辅助对象。
 * - router：API 路由。
 * - method：HTTP 方法。
 * - path：请求路径。
 * - token：可选 Bearer token。
 * - payload：可选 JSON 载荷。
 * 返回值：记录下来的 HTTP 响应。
 * 失败条件：载荷序列化失败时测试失败。
 */
func performJSON(t *testing.T, router *gin.Engine, method string, path string, token string, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatalf("failed to encode payload: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}
