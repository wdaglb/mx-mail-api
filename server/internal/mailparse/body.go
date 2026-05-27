package mailparse

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"

	"golang.org/x/net/html/charset"
)

/**
 * Body 将原始 SMTP DATA 转换为解码后的正文源码。
 *
 * 参数：
 * - raw：已经完成点转义还原后的 SMTP DATA 内容。
 * 返回值：解码后的正文源码。multipart/alternative 邮件优先选择 HTML，因为它最接近发件人原始富文本内容。
 * 当 MIME 解析失败时返回原始内容，确保运维人员仍能检查原始邮件。
 * 失败条件：无；格式异常的邮件会降级为原文文本，而不是让整个 API 响应失败。
 */
func Body(raw string) string {
	return Decode(raw).Body
}

/**
 * Decode 将原始 SMTP DATA 转换为解码后的正文变体。
 *
 * 参数：
 * - raw：已经完成点转义还原后的 SMTP DATA 内容。
 * 返回值：解码后的正文载荷，并将 HTML 与纯文本兜底内容分开返回。
 * 失败条件：格式异常的邮件会在 Body 字段中降级为原文文本。
 */
func Decode(raw string) DecodedBody {
	message, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return DecodedBody{Body: raw}
	}

	contentType := message.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
	}

	decoded, ok, err := readPart(mediaType, params, message.Header.Get("Content-Transfer-Encoding"), message.Body)
	if err != nil || !ok {
		return DecodedBody{Body: raw}
	}

	return decoded
}

/**
 * DecodedBody 保存解码后的邮件正文变体。
 *
 * 字段：
 * - Body：优先展示正文；存在 HTML 时优先使用 HTML。
 * - HTML：邮件包含 HTML 部分时的解码后 HTML 源码。
 * - IsHTML：Body 是否应按 HTML 处理。
 */
type DecodedBody struct {
	Body   string
	HTML   string
	IsHTML bool
}

/**
 * readPart 读取单个 MIME 实体。
 *
 * 参数：
 * - mediaType：解析后的 Content-Type 媒体类型。
 * - params：解析后的 Content-Type 参数。
 * - transferEncoding：Content-Transfer-Encoding 头字段值。
 * - reader：MIME 实体正文读取器。
 * 返回值：解码后的正文，以及是否成功选中了正文部分。
 * 失败条件：正文无法读取或解码失败时返回对应错误。
 */
func readPart(mediaType string, params map[string]string, transferEncoding string, reader io.Reader) (DecodedBody, bool, error) {
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return DecodedBody{}, false, nil
		}

		return readMultipart(multipart.NewReader(reader, boundary))
	}

	decoded, err := decodeTransfer(reader, transferEncoding)
	if err != nil {
		return DecodedBody{}, false, err
	}

	text, err := decodeCharset(decoded, params["charset"])
	if err != nil {
		return DecodedBody{}, false, err
	}

	if strings.HasPrefix(mediaType, "text/html") {
		return DecodedBody{Body: text, HTML: text, IsHTML: true}, true, nil
	}

	return DecodedBody{Body: text}, true, nil
}

/**
 * readMultipart 从 multipart 实体中选择最合适的可读正文。
 *
 * 参数：
 * - reader：multipart 读取器。
 * 返回值：解码后的正文源码，以及是否成功选中正文部分；multipart/alternative 邮件优先选择 text/html。
 * 失败条件：multipart 流格式错误时返回错误。
 */
func readMultipart(reader *multipart.Reader) (DecodedBody, bool, error) {
	var plainFallback DecodedBody
	hasPlainFallback := false

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return DecodedBody{}, false, err
		}

		mediaType, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			mediaType = "text/plain"
		}

		body, ok, err := readPart(mediaType, params, part.Header.Get("Content-Transfer-Encoding"), part)
		if err != nil || !ok {
			continue
		}
		if strings.HasPrefix(mediaType, "text/html") {
			return body, true, nil
		}
		if strings.HasPrefix(mediaType, "text/plain") && !hasPlainFallback {
			plainFallback = body
			hasPlainFallback = true
		}
	}

	return plainFallback, hasPlainFallback, nil
}

/**
 * decodeTransfer 按 Content-Transfer-Encoding 执行传输编码解码。
 *
 * 参数：
 * - reader：编码后的正文读取器。
 * - encoding：Content-Transfer-Encoding 头字段值。
 * 返回值：解码后的字节。
 * 失败条件：读取或解码失败时返回错误。
 */
func decodeTransfer(reader io.Reader, encoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return io.ReadAll(base64.NewDecoder(base64.StdEncoding, reader))
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(reader))
	default:
		return io.ReadAll(reader)
	}
}

/**
 * decodeCharset 将邮件正文字节转换为 UTF-8 文本。
 *
 * 参数：
 * - content：完成传输编码解码后的正文字节。
 * - label：Content-Type 中的 charset 标签。
 * 返回值：UTF-8 文本。
 * 失败条件：字符集解码失败时返回错误。
 */
func decodeCharset(content []byte, label string) (string, error) {
	if label == "" || strings.EqualFold(label, "utf-8") || strings.EqualFold(label, "us-ascii") {
		return string(content), nil
	}

	reader, err := charset.NewReaderLabel(label, bytes.NewReader(content))
	if err != nil {
		return "", err
	}

	decoded, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}
