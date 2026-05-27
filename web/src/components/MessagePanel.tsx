import { Button, Card, Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ApiClient } from '../lib/api';
import { formatDate } from '../lib/format';
import type { MailMessage } from '../types';
import { openMessageBody } from './messageBody';

/**
 * MessagePanel 渲染已接收的 SMTP 邮件记录。
 *
 * 参数：
 * - api：已认证的 API 客户端。
 * - messages：可见收件记录。
 * - loading：表格加载状态。
 * - onRefresh：从 API 重新加载最新记录的回调。
 * 返回值：带正文预览操作的收件记录表格。
 * 失败条件：无；邮件内容只读展示。
 */
export function MessagePanel({
  api,
  messages,
  loading,
  onRefresh,
}: {
  api: ApiClient;
  messages: MailMessage[];
  loading: boolean;
  onRefresh: () => Promise<void>;
}) {
  const columns: ColumnsType<MailMessage> = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: '发件人', dataIndex: 'mail_from' },
    {
      title: '收件人',
      dataIndex: 'rcpt_to',
      render: (values: string[]) => values.join(', '),
    },
    { title: 'HELO/EHLO', dataIndex: 'helo_name' },
    { title: '来源地址', dataIndex: 'remote_addr' },
    {
      title: '接收时间',
      dataIndex: 'created_at',
      render: (value: string) => formatDate(value),
    },
    {
      title: '操作',
      width: 120,
      render: (_, row) => (
        <Button
          onClick={() => {
            void openMessageBody(api, row);
          }}
        >
          查看正文
        </Button>
      ),
    },
  ];

  return (
    <Card
      title="收件记录"
      extra={
        <Button loading={loading} onClick={() => void onRefresh()}>
          刷新
        </Button>
      }
    >
      <Table
        rowKey="id"
        columns={columns}
        dataSource={messages}
        loading={loading}
        pagination={{ pageSize: 10 }}
      />
    </Card>
  );
}
