import { Modal, message } from 'antd';
import type { ApiClient } from '../lib/api';
import { errorMessage } from '../lib/errors';
import type { MailMessage, MessageBody } from '../types';

/**
 * openMessageBody 按需拉取并展示解码后的邮件正文。
 *
 * 参数：
 * - api：已认证的 API 客户端。
 * - row：选中的邮件列表行。
 * 返回值：无。
 * 失败条件：API 失败会通过 Ant Design message 展示。
 */
export async function openMessageBody(api: ApiClient, row: MailMessage) {
  try {
    const data = await api.get<{ item: MessageBody }>(
      `/api/messages/${row.id}/body`,
    );
    const html = data.item.html || (data.item.is_html ? data.item.body : '');
    Modal.info({
      title: `邮件 #${row.id}`,
      width: 760,
      content: html ? (
        <iframe
          className="message-html-preview"
          sandbox=""
          srcDoc={html}
          title={`邮件 #${row.id} HTML 正文`}
        />
      ) : (
        <pre className="message-preview">
          {data.item.body || data.item.data}
        </pre>
      ),
    });
  } catch (error) {
    message.error(errorMessage(error));
  }
}
