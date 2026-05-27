package mailparse

import "testing"

/**
 * TestBodyDecodesQuotedPrintable 校验常见 quoted-printable 文本正文会还原为可读文本。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：quoted-printable 解码能力退化时测试失败。
 */
func TestBodyDecodesQuotedPrintable(t *testing.T) {
	raw := "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nhello=20world=0A"

	if got := Body(raw); got != "hello world\n" {
		t.Fatalf("expected decoded quoted-printable body, got %q", got)
	}
}

/**
 * TestBodyPrefersMultipartHTML 校验 multipart 邮件优先选择 HTML 内容。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：multipart 遍历不再选择 HTML 正文源码时测试失败。
 */
func TestBodyPrefersMultipartHTML(t *testing.T) {
	raw := "Content-Type: multipart/alternative; boundary=abc\r\n\r\n" +
		"--abc\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nplain body\r\n" +
		"--abc\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<b>html body</b>\r\n" +
		"--abc--\r\n"

	if got := Body(raw); got != "<b>html body</b>" {
		t.Fatalf("expected html body source, got %q", got)
	}
	if decoded := Decode(raw); !decoded.IsHTML || decoded.HTML != "<b>html body</b>" {
		t.Fatalf("expected decoded html body, got %#v", decoded)
	}
}

/**
 * TestBodyReturnsHTMLSource 校验纯 HTML 邮件保留解码后的 HTML 源码。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：HTML 正文源码被剥离或改写时测试失败。
 */
func TestBodyReturnsHTMLSource(t *testing.T) {
	raw := "Content-Type: text/html; charset=utf-8\r\n\r\n<p>Hello&nbsp;<b>world</b></p>"

	if got := Body(raw); got != "<p>Hello&nbsp;<b>world</b></p>" {
		t.Fatalf("expected html source, got %q", got)
	}
}

/**
 * TestBodyPrefersHTMLWhenPlainIsBlank 校验 text/plain 为空白行时仍选择 HTML。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：真实邮件服务商的 multipart 邮件退化为空白 plain 正文或原始 MIME DATA 时测试失败。
 */
func TestBodyPrefersHTMLWhenPlainIsBlank(t *testing.T) {
	raw := "DKIM-Signature: v=1; a=rsa-sha256; d=163.com\r\n" +
		"Content-Type: multipart/alternative; boundary=\"----=_Part_159273_1529317577.1779807211824\"\r\n" +
		"MIME-Version: 1.0\r\n\r\n" +
		"------=_Part_159273_1529317577.1779807211824\r\n" +
		"Content-Type: text/plain; charset=GBK\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		"Cg==\r\n" +
		"------=_Part_159273_1529317577.1779807211824\r\n" +
		"Content-Type: text/html; charset=GBK\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		"PGRpdj48YnI+PC9kaXY+\r\n" +
		"------=_Part_159273_1529317577.1779807211824--\r\n"

	if got := Body(raw); got != "<div><br></div>" {
		t.Fatalf("expected decoded html source, got %q", got)
	}
}
