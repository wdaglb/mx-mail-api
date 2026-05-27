package smtpserver

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"mx-mail-api/internal/repository"
	"mx-mail-api/internal/service"
	"mx-mail-api/internal/storage"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

/**
 * TestHandleConnAcceptsBasicMessage 校验已申请临时邮箱的 SMTP 收件流程。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：命令顺序、DATA 解析或邮件持久化行为退化时测试失败。
 */
func TestHandleConnAcceptsBasicMessage(t *testing.T) {
	messages, db := newTestMessageService(t, []string{"example.test"})
	seedActiveMailbox(t, db, "bob@example.test")
	server := &Server{
		Hostname: "mx.test",
		Messages: messages,
	}

	client, serverConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- server.HandleConn(context.Background(), serverConn)
	}()

	reader := bufio.NewReader(client)
	expectLine(t, reader, "220 mx.test ESMTP mx-mail-api ready")
	writeLine(t, client, "EHLO sender.test")
	expectLine(t, reader, "250-mx.test")
	expectLine(t, reader, "250 HELP")
	writeLine(t, client, "MAIL FROM:<alice@example.test>")
	expectLine(t, reader, "250 Sender accepted")
	writeLine(t, client, "RCPT TO:<bob@example.test>")
	expectLine(t, reader, "250 Recipient accepted")
	writeLine(t, client, "DATA")
	expectLine(t, reader, "354 End data with <CR><LF>.<CR><LF>")
	writeLine(t, client, "Subject: Hello")
	writeLine(t, client, "")
	writeLine(t, client, "..escaped")
	writeLine(t, client, ".")
	expectLine(t, reader, "250 Message accepted")
	writeLine(t, client, "QUIT")
	expectLine(t, reader, "221 Bye")
	_ = client.Close()

	if err := <-done; err != nil {
		t.Fatalf("expected clean SMTP session, got %v", err)
	}

	var stored []storage.Message
	if err := db.Find(&stored).Error; err != nil {
		t.Fatalf("failed to query stored messages: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored message, got %d", len(stored))
	}

	message := stored[0]
	if message.HeloName != "sender.test" {
		t.Fatalf("expected helo sender.test, got %s", message.HeloName)
	}
	if message.MailFrom != "alice@example.test" {
		t.Fatalf("expected sender alice@example.test, got %s", message.MailFrom)
	}
	if len(message.RcptTo) != 1 || message.RcptTo[0] != "bob@example.test" {
		t.Fatalf("expected recipient bob@example.test, got %#v", message.RcptTo)
	}
	if message.Data != "Subject: Hello\n\n.escaped\n" {
		t.Fatalf("unexpected DATA body: %q", message.Data)
	}
}

/**
 * TestHandleConnRejectsInvalidSequence 校验信封未完整前不能执行 DATA。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：服务端在 MAIL/RCPT 前接受 DATA 时测试失败。
 */
func TestHandleConnRejectsInvalidSequence(t *testing.T) {
	messages, db := newTestMessageService(t, []string{"example.test"})
	server := &Server{
		Hostname: "mx.test",
		Messages: messages,
	}

	client, serverConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- server.HandleConn(context.Background(), serverConn)
	}()

	reader := bufio.NewReader(client)
	expectLine(t, reader, "220 mx.test ESMTP mx-mail-api ready")
	writeLine(t, client, "DATA")
	expectLine(t, reader, "503 Need MAIL FROM and RCPT TO before DATA")
	writeLine(t, client, "QUIT")
	expectLine(t, reader, "221 Bye")
	_ = client.Close()

	if err := <-done; err != nil {
		t.Fatalf("expected clean SMTP session, got %v", err)
	}
	var count int64
	if err := db.Model(&storage.Message{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count stored messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no stored messages, got %d", count)
	}
}

/**
 * TestHandleConnAcceptsSubdomainRecipientDomain 校验 RCPT TO 阶段配置根域名即可接受子域名。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件："example.test" 不再接受根域名或子域名时测试失败。
 */
func TestHandleConnAcceptsSubdomainRecipientDomain(t *testing.T) {
	messages, db := newTestMessageService(t, []string{"example.test"})
	seedActiveMailbox(t, db, "bob@team.example.test")
	server := &Server{
		Hostname: "mx.test",
		Messages: messages,
	}

	client, serverConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- server.HandleConn(context.Background(), serverConn)
	}()

	reader := bufio.NewReader(client)
	expectLine(t, reader, "220 mx.test ESMTP mx-mail-api ready")
	writeLine(t, client, "HELO sender.test")
	expectLine(t, reader, "250 mx.test")
	writeLine(t, client, "MAIL FROM:<alice@example.test>")
	expectLine(t, reader, "250 Sender accepted")
	writeLine(t, client, "RCPT TO:<bob@team.example.test>")
	expectLine(t, reader, "250 Recipient accepted")
	writeLine(t, client, "QUIT")
	expectLine(t, reader, "221 Bye")
	_ = client.Close()

	if err := <-done; err != nil {
		t.Fatalf("expected clean SMTP session, got %v", err)
	}
}

/**
 * TestHandleConnRejectsUnleasedTemporaryMailbox 校验未申请临时邮箱时 SMTP 直接拒收。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：未租赁地址仍被 RCPT TO 接受或产生入库记录时测试失败。
 */
func TestHandleConnRejectsUnleasedTemporaryMailbox(t *testing.T) {
	messages, db := newTestMessageService(t, []string{"example.test"})
	server := &Server{
		Hostname: "mx.test",
		Messages: messages,
	}

	client, serverConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- server.HandleConn(context.Background(), serverConn)
	}()

	reader := bufio.NewReader(client)
	expectLine(t, reader, "220 mx.test ESMTP mx-mail-api ready")
	writeLine(t, client, "HELO sender.test")
	expectLine(t, reader, "250 mx.test")
	writeLine(t, client, "MAIL FROM:<alice@example.test>")
	expectLine(t, reader, "250 Sender accepted")
	writeLine(t, client, "RCPT TO:<bob@example.test>")
	expectLine(t, reader, "550 Relay denied")
	writeLine(t, client, "QUIT")
	expectLine(t, reader, "221 Bye")
	_ = client.Close()

	if err := <-done; err != nil {
		t.Fatalf("expected clean SMTP session, got %v", err)
	}
	var count int64
	if err := db.Model(&storage.Message{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count stored messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no stored messages, got %d", count)
	}
}

/**
 * TestHandleConnRejectsUnacceptedRecipientDomain 校验域名策略会在 DATA 前拒绝无关 RCPT TO 域名。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：服务端接受配置域名体系之外的转发目标时测试失败。
 */
func TestHandleConnRejectsUnacceptedRecipientDomain(t *testing.T) {
	messages, db := newTestMessageService(t, []string{"example.test"})
	server := &Server{
		Hostname: "mx.test",
		Messages: messages,
	}

	client, serverConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- server.HandleConn(context.Background(), serverConn)
	}()

	reader := bufio.NewReader(client)
	expectLine(t, reader, "220 mx.test ESMTP mx-mail-api ready")
	writeLine(t, client, "HELO sender.test")
	expectLine(t, reader, "250 mx.test")
	writeLine(t, client, "MAIL FROM:<alice@example.test>")
	expectLine(t, reader, "250 Sender accepted")
	writeLine(t, client, "RCPT TO:<bob@other.test>")
	expectLine(t, reader, "550 Relay denied")
	writeLine(t, client, "QUIT")
	expectLine(t, reader, "221 Bye")
	_ = client.Close()

	if err := <-done; err != nil {
		t.Fatalf("expected clean SMTP session, got %v", err)
	}
	var count int64
	if err := db.Model(&storage.Message{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count stored messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no stored messages, got %d", count)
	}
}

/**
 * TestHandleConnRejectsExpiredTemporaryMailbox 校验临时邮箱过期后 SMTP 拒收。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：过期临时邮箱仍被 RCPT TO 接受时测试失败。
 */
func TestHandleConnRejectsExpiredTemporaryMailbox(t *testing.T) {
	messages, db := newTestMessageService(t, []string{"example.test"})
	seedMailbox(t, db, "tmp-expired@example.test", time.Now().Add(-time.Minute))

	server := &Server{
		Hostname: "mx.test",
		Messages: messages,
	}

	client, serverConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- server.HandleConn(context.Background(), serverConn)
	}()

	reader := bufio.NewReader(client)
	expectLine(t, reader, "220 mx.test ESMTP mx-mail-api ready")
	writeLine(t, client, "HELO sender.test")
	expectLine(t, reader, "250 mx.test")
	writeLine(t, client, "MAIL FROM:<alice@example.test>")
	expectLine(t, reader, "250 Sender accepted")
	writeLine(t, client, "RCPT TO:<tmp-expired@example.test>")
	expectLine(t, reader, "550 Relay denied")
	writeLine(t, client, "QUIT")
	expectLine(t, reader, "221 Bye")
	_ = client.Close()

	if err := <-done; err != nil {
		t.Fatalf("expected clean SMTP session, got %v", err)
	}
}

/**
 * TestDomainMatches 固化根域名和子域名收件匹配语义。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：根域名不再匹配自身和子域名，或旧 "*" 通配仍然生效时测试失败。
 */
func TestDomainMatches(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		domain  string
		want    bool
	}{
		{name: "exact", pattern: "example.test", domain: "example.test", want: true},
		{name: "exact ignores case", pattern: "Example.Test.", domain: "example.test", want: true},
		{name: "root accepts subdomain", pattern: "example.test", domain: "team.example.test", want: true},
		{name: "root accepts nested subdomain", pattern: "example.test", domain: "a.team.example.test", want: true},
		{name: "wildcard no longer matches subdomain", pattern: "*.example.test", domain: "team.example.test", want: false},
		{name: "wildcard no longer matches root", pattern: "*.example.test", domain: "example.test", want: false},
		{name: "unrelated", pattern: "example.test", domain: "other.test", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := storage.DomainMatches(tc.pattern, tc.domain); got != tc.want {
				t.Fatalf("DomainMatches(%q, %q) = %v, want %v", tc.pattern, tc.domain, got, tc.want)
			}
		})
	}
}

/**
 * newTestMessageService 创建使用内存 SQLite 的收件业务服务。
 *
 * 参数：
 * - t：测试辅助对象。
 * - patterns：预置接受域名规则。
 * 返回值：收件业务服务和底层数据库句柄。
 * 失败条件：数据库打开、迁移或预置数据失败时测试失败。
 */
func newTestMessageService(t *testing.T, patterns []string) (*service.MessageService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&storage.User{}, &storage.AcceptedDomain{}, &storage.TemporaryMailbox{}, &storage.Message{}); err != nil {
		t.Fatalf("failed to migrate sqlite schema: %v", err)
	}

	for _, pattern := range patterns {
		domain := storage.AcceptedDomain{Domain: storage.NormalizeDomain(pattern)}
		if err := db.Create(&domain).Error; err != nil {
			t.Fatalf("failed to seed accepted domain %q: %v", pattern, err)
		}
	}

	messageRepo := repository.NewMessageRepository(db)
	domainRepo := repository.NewDomainRepository(db)
	temporaryMailboxRepo := repository.NewTemporaryMailboxRepository(db)
	temporaryMailboxService := service.NewTemporaryMailboxService(temporaryMailboxRepo, domainRepo)
	return service.NewMessageService(messageRepo, domainRepo, temporaryMailboxService), db
}

/**
 * seedActiveMailbox 写入一个未过期的临时邮箱测试数据。
 *
 * 参数：
 * - t：测试辅助对象。
 * - db：测试数据库。
 * - address：完整临时邮箱地址。
 * 返回值：无。
 * 失败条件：测试数据插入失败时测试失败。
 */
func seedActiveMailbox(t *testing.T, db *gorm.DB, address string) {
	t.Helper()
	seedMailbox(t, db, address, time.Now().Add(time.Hour))
}

/**
 * seedMailbox 写入指定过期时间的临时邮箱测试数据。
 *
 * 参数：
 * - t：测试辅助对象。
 * - db：测试数据库。
 * - address：完整临时邮箱地址。
 * - expiresAt：临时邮箱过期时间。
 * 返回值：无。
 * 失败条件：测试用户或临时邮箱插入失败时测试失败。
 */
func seedMailbox(t *testing.T, db *gorm.DB, address string, expiresAt time.Time) {
	t.Helper()

	var user storage.User
	if err := db.FirstOrCreate(&user, storage.User{
		Username: "alice",
	}, storage.User{
		PasswordHash: "hash",
		Role:         storage.RoleUser,
	}).Error; err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		t.Fatalf("invalid mailbox address %q", address)
	}

	if err := db.Create(&storage.TemporaryMailbox{
		Address:     address,
		LocalPart:   address[:at],
		Domain:      address[at+1:],
		OwnerUserID: user.ID,
		ExpiresAt:   expiresAt,
	}).Error; err != nil {
		t.Fatalf("failed to seed temporary mailbox: %v", err)
	}
}

/**
 * writeLine 向测试连接发送一行 SMTP 内容。
 *
 * 参数：
 * - t：测试辅助对象。
 * - conn：客户端侧 pipe 连接。
 * - line：不包含 CRLF 的命令或 DATA 行。
 * 返回值：无。
 * 失败条件：写入失败时测试失败。
 */
func writeLine(t *testing.T, conn net.Conn, line string) {
	t.Helper()

	if _, err := conn.Write([]byte(line + "\r\n")); err != nil {
		t.Fatalf("failed to write line %q: %v", line, err)
	}
}

/**
 * expectLine 读取并比较一行 SMTP 响应。
 *
 * 参数：
 * - t：测试辅助对象。
 * - reader：客户端侧缓冲读取器。
 * - expected：不包含 CRLF 的预期响应。
 * 返回值：无。
 * 失败条件：读取失败或响应不一致时测试失败。
 */
func expectLine(t *testing.T, reader *bufio.Reader, expected string) {
	t.Helper()

	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read response %q: %v", expected, err)
	}

	actual := strings.TrimRight(line, "\r\n")
	if actual != expected {
		t.Fatalf("expected response %q, got %q", expected, actual)
	}
}
