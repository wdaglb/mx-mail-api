package smtpserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"mx-mail-api/internal/service"
)

const (
	defaultReadTimeout  = 5 * time.Minute
	defaultWriteTimeout = 30 * time.Second
)

/**
 * Server 接受 SMTP TCP 连接并存储成功接收的邮件。
 *
 * 字段：
 * - Addr：TCP 监听地址，例如 ":2525"。
 * - Hostname：greeting 和 EHLO 响应中发送的服务端身份。
 * - Messages：DATA 完成后使用的收件业务服务。
 * - ReadTimeout：单条命令读取超时时间。
 * - WriteTimeout：响应写入超时时间。
 */
type Server struct {
	Addr         string
	Hostname     string
	Messages     *service.MessageService
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

/**
 * ListenAndServe 打开配置的 TCP 监听器并处理 SMTP 会话。
 *
 * 参数：
 * - ctx：进程取消上下文。
 * 返回值：监听器无法启动或异常退出时返回错误。
 * 失败条件：地址非法、端口冲突、Messages 为空，或出现非上下文取消导致的监听错误。
 */
func (server *Server) ListenAndServe(ctx context.Context) error {
	if server.Messages == nil {
		return errors.New("smtp server requires a message service")
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Printf("smtp listen failed addr=%q error=%q", server.Addr, err.Error())
		return err
	}
	defer listener.Close()
	log.Printf("smtp listen started addr=%q hostname=%q", listener.Addr().String(), server.hostname())

	go func() {
		<-ctx.Done()
		// 关闭 listener 是解除 Accept 阻塞的最直接方式，避免额外引入自定义控制通道。
		_ = listener.Close()
	}()

	return server.Serve(ctx, listener)
}

/**
 * Serve 使用已有 listener 接受连接。
 *
 * 参数：
 * - ctx：进程取消上下文。
 * - listener：已经打开的 TCP listener。
 * 返回值：由上下文驱动关闭时返回 nil，否则返回 Accept 错误。
 * 失败条件：Accept 异常失败时返回错误。
 */
func (server *Server) Serve(ctx context.Context, listener net.Listener) error {
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				log.Printf("smtp listen stopped addr=%q", listener.Addr().String())
				return nil
			}

			log.Printf("smtp accept failed addr=%q error=%q", listener.Addr().String(), err.Error())
			return err
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := server.HandleConn(ctx, conn); err != nil {
				log.Printf("smtp session ended error=%q", err.Error())
			}
		}()
	}
}

/**
 * HandleConn 处理单个 SMTP 会话。
 *
 * 参数：
 * - ctx：进程取消上下文。
 * - conn：已接受的 TCP 连接。
 * 返回值：正常 QUIT/EOF 时返回 nil，否则返回连接或命令错误。
 * 失败条件：写入、读取或邮件持久化失败时返回错误。
 */
func (server *Server) HandleConn(ctx context.Context, conn net.Conn) error {
	defer conn.Close()

	session := newSession(server, conn.RemoteAddr().String())
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	log.Printf("smtp connection opened remote=%q local=%q", session.remoteAddr, conn.LocalAddr().String())
	defer func(startedAt time.Time) {
		log.Printf("smtp connection closed remote=%q helo=%q duration_ms=%d", session.remoteAddr, session.heloName, time.Since(startedAt).Milliseconds())
	}(time.Now())

	if err := session.writeLine(conn, writer, "220 "+server.hostname()+" ESMTP mx-mail-api ready"); err != nil {
		return err
	}

	for {
		if deadlineErr := server.applyReadDeadline(conn); deadlineErr != nil {
			return deadlineErr
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		shouldClose, err := session.handleLine(ctx, conn, reader, writer, strings.TrimRight(line, "\r\n"))
		if err != nil || shouldClose {
			return err
		}
	}
}

/**
 * hostname 返回非空 SMTP 主机名。
 *
 * 参数：无。
 * 返回值：配置的主机名，或本地兜底主机名。
 * 失败条件：无。
 */
func (server *Server) hostname() string {
	if server.Hostname != "" {
		return server.Hostname
	}

	return "mx-mail-api.local"
}

/**
 * applyReadDeadline 在配置存在时设置单命令读取截止时间。
 *
 * 参数：
 * - conn：活跃 SMTP 连接。
 * 返回值：连接实现返回的截止时间设置错误。
 * 失败条件：SetReadDeadline 失败时返回错误。
 */
func (server *Server) applyReadDeadline(conn net.Conn) error {
	timeout := server.ReadTimeout
	if timeout == 0 {
		timeout = defaultReadTimeout
	}

	return conn.SetReadDeadline(time.Now().Add(timeout))
}

/**
 * applyWriteDeadline 在配置存在时设置单响应写入截止时间。
 *
 * 参数：
 * - conn：活跃 SMTP 连接。
 * 返回值：连接实现返回的截止时间设置错误。
 * 失败条件：SetWriteDeadline 失败时返回错误。
 */
func (server *Server) applyWriteDeadline(conn net.Conn) error {
	timeout := server.WriteTimeout
	if timeout == 0 {
		timeout = defaultWriteTimeout
	}

	return conn.SetWriteDeadline(time.Now().Add(timeout))
}

type smtpSession struct {
	server     *Server
	heloName   string
	mailFrom   string
	rcptTo     []string
	remoteAddr string
}

/**
 * newSession 为单个 SMTP 连接创建会话状态。
 *
 * 参数：
 * - server：所属 SMTP 服务。
 * - remoteAddr：TCP 对端地址。
 * 返回值：已初始化的会话状态。
 * 失败条件：无。
 */
func newSession(server *Server, remoteAddr string) *smtpSession {
	return &smtpSession{
		server:     server,
		remoteAddr: remoteAddr,
	}
}

/**
 * handleLine 分发一行 SMTP 命令。
 *
 * 参数：
 * - ctx：进程取消上下文。
 * - conn：活跃 SMTP 连接。
 * - reader：DATA 模式使用的缓冲读取器。
 * - writer：SMTP 响应使用的缓冲写入器。
 * - line：不包含 CRLF 的命令行。
 * 返回值：是否应关闭连接；发生致命连接或存储错误时同时返回错误。
 * 失败条件：写入失败或 DATA 持久化失败时返回错误。
 */
func (session *smtpSession) handleLine(ctx context.Context, conn net.Conn, reader *bufio.Reader, writer *bufio.Writer, line string) (bool, error) {
	command, argument := splitCommand(line)

	switch command {
	case "HELO", "EHLO":
		session.resetEnvelope()
		session.heloName = strings.TrimSpace(argument)
		if session.heloName == "" {
			log.Printf("smtp helo rejected remote=%q command=%q reason=%q", session.remoteAddr, command, "empty_hostname")
			return false, session.writeLine(conn, writer, "501 HELO/EHLO requires a hostname")
		}

		log.Printf("smtp helo accepted remote=%q command=%q helo=%q", session.remoteAddr, command, session.heloName)
		if command == "EHLO" {
			// 多行 EHLO 响应为后续能力扩展保留标准格式；当前 MVP 不声明 AUTH/TLS 等未实现能力。
			if err := session.writeLine(conn, writer, "250-"+session.server.hostname()); err != nil {
				return false, err
			}
			return false, session.writeLine(conn, writer, "250 HELP")
		}

		return false, session.writeLine(conn, writer, "250 "+session.server.hostname())
	case "MAIL":
		if session.heloName == "" {
			log.Printf("smtp mail_from rejected remote=%q reason=%q", session.remoteAddr, "missing_helo")
			return false, session.writeLine(conn, writer, "503 Send HELO/EHLO first")
		}

		address, ok := parseAddressArgument(argument, "FROM:")
		if !ok {
			log.Printf("smtp mail_from rejected remote=%q helo=%q reason=%q argument=%q", session.remoteAddr, session.heloName, "invalid_address", argument)
			return false, session.writeLine(conn, writer, "501 MAIL requires FROM:<address>")
		}

		session.mailFrom = address
		session.rcptTo = nil
		log.Printf("smtp mail_from accepted remote=%q helo=%q from=%q", session.remoteAddr, session.heloName, session.mailFrom)
		return false, session.writeLine(conn, writer, "250 Sender accepted")
	case "RCPT":
		if session.mailFrom == "" {
			log.Printf("smtp rcpt_to rejected remote=%q helo=%q reason=%q", session.remoteAddr, session.heloName, "missing_mail_from")
			return false, session.writeLine(conn, writer, "503 Send MAIL FROM first")
		}

		address, ok := parseAddressArgument(argument, "TO:")
		if !ok {
			log.Printf("smtp rcpt_to rejected remote=%q helo=%q from=%q reason=%q argument=%q", session.remoteAddr, session.heloName, session.mailFrom, "invalid_address", argument)
			return false, session.writeLine(conn, writer, "501 RCPT requires TO:<address>")
		}
		accepted, err := session.server.Messages.AcceptsRecipient(ctx, address)
		if errors.Is(err, service.ErrTemporaryMailboxExpired) || errors.Is(err, service.ErrTemporaryMailboxNotFound) {
			log.Printf("smtp rcpt_to rejected remote=%q helo=%q from=%q to=%q reason=%q", session.remoteAddr, session.heloName, session.mailFrom, address, temporaryMailboxRejectReason(err))
			return false, session.writeLine(conn, writer, "550 Relay denied")
		}
		if err != nil {
			log.Printf("smtp rcpt_to policy_error remote=%q helo=%q from=%q to=%q error=%q", session.remoteAddr, session.heloName, session.mailFrom, address, err.Error())
			return false, session.writeLine(conn, writer, "451 Requested action aborted: local error in processing")
		}
		if !accepted {
			log.Printf("smtp rcpt_to rejected remote=%q helo=%q from=%q to=%q reason=%q", session.remoteAddr, session.heloName, session.mailFrom, address, "domain_not_accepted")
			return false, session.writeLine(conn, writer, "550 Relay denied")
		}

		session.rcptTo = append(session.rcptTo, address)
		log.Printf("smtp rcpt_to accepted remote=%q helo=%q from=%q to=%q", session.remoteAddr, session.heloName, session.mailFrom, address)
		return false, session.writeLine(conn, writer, "250 Recipient accepted")
	case "DATA":
		if session.mailFrom == "" || len(session.rcptTo) == 0 {
			log.Printf("smtp data rejected remote=%q helo=%q from=%q rcpt_count=%d reason=%q", session.remoteAddr, session.heloName, session.mailFrom, len(session.rcptTo), "missing_envelope")
			return false, session.writeLine(conn, writer, "503 Need MAIL FROM and RCPT TO before DATA")
		}

		log.Printf("smtp data started remote=%q helo=%q from=%q rcpt_count=%d", session.remoteAddr, session.heloName, session.mailFrom, len(session.rcptTo))
		if err := session.writeLine(conn, writer, "354 End data with <CR><LF>.<CR><LF>"); err != nil {
			return false, err
		}

		data, err := readData(reader)
		if err != nil {
			log.Printf("smtp data read failed remote=%q helo=%q from=%q rcpt_count=%d error=%q", session.remoteAddr, session.heloName, session.mailFrom, len(session.rcptTo), err.Error())
			return false, err
		}

		if _, err := session.server.Messages.SaveReceived(ctx, service.ReceivedMessageInput{
			HeloName:   session.heloName,
			MailFrom:   session.mailFrom,
			RcptTo:     session.rcptTo,
			Data:       data,
			RemoteAddr: session.remoteAddr,
		}); err != nil {
			log.Printf("smtp message_store failed remote=%q helo=%q from=%q rcpt_count=%d size_bytes=%d error=%q", session.remoteAddr, session.heloName, session.mailFrom, len(session.rcptTo), len(data), err.Error())
			return false, session.writeLine(conn, writer, "451 Requested action aborted: local error in processing")
		}

		log.Printf("smtp message_store accepted remote=%q helo=%q from=%q rcpt_count=%d size_bytes=%d", session.remoteAddr, session.heloName, session.mailFrom, len(session.rcptTo), len(data))
		session.resetEnvelope()
		return false, session.writeLine(conn, writer, "250 Message accepted")
	case "RSET":
		session.resetEnvelope()
		return false, session.writeLine(conn, writer, "250 Reset ok")
	case "NOOP":
		return false, session.writeLine(conn, writer, "250 Ok")
	case "QUIT":
		if err := session.writeLine(conn, writer, "221 Bye"); err != nil {
			return true, err
		}
		return true, nil
	default:
		return false, session.writeLine(conn, writer, "502 Command not implemented")
	}
}

/**
 * resetEnvelope 清空单封邮件状态，同时保留会话 greeting 信息。
 *
 * 参数：无。
 * 返回值：无。
 * 失败条件：无。
 */
func (session *smtpSession) resetEnvelope() {
	session.mailFrom = ""
	session.rcptTo = nil
}

/**
 * writeLine 写入一行 SMTP 响应并立即刷新。
 *
 * 参数：
 * - conn：活跃 SMTP 连接。
 * - writer：响应缓冲写入器。
 * - line：不包含 CRLF 的响应内容。
 * 返回值：响应行刷新成功时返回 nil。
 * 失败条件：截止时间设置、写入或刷新失败时返回错误。
 */
func (session *smtpSession) writeLine(conn net.Conn, writer *bufio.Writer, line string) error {
	if err := session.server.applyWriteDeadline(conn); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(writer, "%s\r\n", line); err != nil {
		return err
	}

	return writer.Flush()
}

/**
 * splitCommand 拆分 SMTP 命令动词和参数。
 *
 * 参数：
 * - line：不包含 CRLF 的原始命令行。
 * 返回值：大写命令和去除首尾空白后的参数。
 * 失败条件：无。
 */
func splitCommand(line string) (string, string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", ""
	}

	parts := strings.SplitN(trimmed, " ", 2)
	command := strings.ToUpper(parts[0])
	if len(parts) == 1 {
		return command, ""
	}

	return command, strings.TrimSpace(parts[1])
}

/**
 * parseAddressArgument 解析 MAIL FROM 和 RCPT TO 参数。
 *
 * 参数：
 * - argument：SMTP 命令参数。
 * - prefix：预期参数前缀，例如 "FROM:" 或 "TO:"。
 * 返回值：参数有效时返回解析出的地址和 true，否则返回空字符串和 false。
 * 失败条件：无；MVP 阶段会忽略不支持的 SMTP 扩展，避免半支持导致行为不一致。
 */
func parseAddressArgument(argument string, prefix string) (string, bool) {
	if !strings.HasPrefix(strings.ToUpper(argument), prefix) {
		return "", false
	}

	value := strings.TrimSpace(argument[len(prefix):])
	if strings.HasPrefix(value, "<") {
		end := strings.Index(value, ">")
		if end <= 1 {
			return "", false
		}

		return value[1:end], true
	}

	if value == "" {
		return "", false
	}

	return strings.Fields(value)[0], true
}

/**
 * temporaryMailboxRejectReason 将临时邮箱拒收错误转换为日志原因。
 *
 * 参数：
 * - err：临时邮箱校验错误。
 * 返回值：用于结构化日志的拒收原因。
 * 失败条件：无；未知错误会返回通用原因，避免日志为空。
 */
func temporaryMailboxRejectReason(err error) string {
	if errors.Is(err, service.ErrTemporaryMailboxExpired) {
		return "temporary_mailbox_expired"
	}
	if errors.Is(err, service.ErrTemporaryMailboxNotFound) {
		return "temporary_mailbox_not_found"
	}

	return "temporary_mailbox_rejected"
}

/**
 * readData 读取 SMTP DATA 内容，直到遇到单独的点号行。
 *
 * 参数：
 * - reader：SMTP 连接缓冲读取器。
 * 返回值：已经还原 SMTP 点转义的 DATA 正文。
 * 失败条件：读取失败或在 DATA 结束符前遇到 EOF 时返回错误。
 */
func readData(reader *bufio.Reader) (string, error) {
	var builder strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "." {
			return builder.String(), nil
		}

		if strings.HasPrefix(trimmed, "..") {
			trimmed = trimmed[1:]
		}

		// 统一保存为 LF，避免不同客户端 CRLF/LF 差异污染后续查看与测试断言。
		builder.WriteString(trimmed)
		builder.WriteByte('\n')
	}
}
