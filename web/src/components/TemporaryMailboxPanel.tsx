import {
  Button,
  Card,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useEffect, useMemo, useState } from 'react';
import type { ApiClient } from '../lib/api';
import { errorMessage } from '../lib/errors';
import { formatDate, formatTTLMinutes } from '../lib/format';
import {
  isTemporaryMailboxSelectableDomain,
  normalizeTemporaryMailboxTTLMinutes,
} from '../lib/temporaryMailbox';
import type {
  Domain,
  MailMessage,
  TemporaryMailbox,
  TemporaryMailboxCreateResult,
  TemporaryMailboxFormValues,
  User,
} from '../types';
import { openMessageBody } from './messageBody';

const permanentTimeValue = -1;

/**
 * TemporaryMailboxPanel 渲染临时邮箱申请入口和历史列表。
 *
 * 参数：
 * - api：已认证的 API 客户端。
 * - currentUser：当前认证用户，用于渲染该用户可选租赁时间。
 * - domains：当前用户可用域名。
 * - loading：全局加载状态。
 * - mailboxes：当前用户申请过的临时邮箱。
 * - messages：当前用户可见的收件记录，用于在临时邮箱弹窗内按收件人筛选展示。
 * - onChanged：申请成功后的刷新回调。
 * 返回值：临时邮箱申请面板。
 * 失败条件：申请失败会通过 Ant Design message 展示。
 */
export function TemporaryMailboxPanel({
  api,
  currentUser,
  domains,
  loading,
  mailboxes,
  messages,
  onChanged,
}: {
  api: ApiClient;
  currentUser: User;
  domains: Domain[];
  loading: boolean;
  mailboxes: TemporaryMailbox[];
  messages: MailMessage[];
  onChanged: () => Promise<void>;
}) {
  const [form] = Form.useForm<TemporaryMailboxFormValues>();
  const [submitting, setSubmitting] = useState(false);
  const [inboxMailbox, setInboxMailbox] = useState<TemporaryMailbox | null>(
    null,
  );
  const [mailboxQuery, setMailboxQuery] = useState('');
  const ttlOptions = useMemo(
    () =>
      normalizeTemporaryMailboxTTLMinutes(
        currentUser.temporary_mailbox_ttl_minutes,
      ),
    [currentUser.temporary_mailbox_ttl_minutes],
  );
  const selectableDomains = useMemo(
    () =>
      domains.filter((domain) =>
        isTemporaryMailboxSelectableDomain(domain.domain, domain.disabled),
      ),
    [domains],
  );
  const inboxMessages = useMemo(() => {
    if (!inboxMailbox) {
      return [];
    }

    const address = inboxMailbox.address.toLowerCase();
    // 一封 SMTP 邮件可能有多个 RCPT TO，这里只展示精确发给当前临时邮箱的记录。
    return messages.filter((item) =>
      item.rcpt_to.some((recipient) => recipient.toLowerCase() === address),
    );
  }, [inboxMailbox, messages]);
  const filteredMailboxes = useMemo(() => {
    const keyword = mailboxQuery.trim().toLowerCase();
    if (!keyword) {
      return mailboxes;
    }

    // 查询只作用于当前用户已经可见的邮箱列表，避免额外引入后端权限分支。
    return mailboxes.filter(
      (mailbox) =>
        mailbox.address.toLowerCase().includes(keyword) ||
        mailbox.domain.toLowerCase().includes(keyword),
    );
  }, [mailboxQuery, mailboxes]);

  useEffect(() => {
    // 用户资料刷新后同步默认租赁时间，避免管理员调整配置后表单仍保留旧值。
    form.setFieldsValue({ ttl_minutes: ttlOptions[0] });
  }, [form, ttlOptions]);

  const create = async (values: TemporaryMailboxFormValues) => {
    setSubmitting(true);
    try {
      const permanent = values.ttl_minutes === permanentTimeValue;
      const data = await api.post<{ item: TemporaryMailboxCreateResult }>(
        '/api/temporary-mailboxes',
        {
          domain: values.domain,
          local_part: values.local_part?.trim() || undefined,
          permanent,
          ttl_minutes: permanent ? undefined : values.ttl_minutes,
        },
      );
      message.success(
        data.item.is_permanent
          ? '永久邮箱已生成'
          : `临时邮箱已生成，有效 ${data.item.ttl_minutes} 分钟`,
      );
      form.resetFields();
      await onChanged();
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSubmitting(false);
    }
  };

  const columns: ColumnsType<TemporaryMailbox> = [
    {
      title: '邮箱地址',
      dataIndex: 'address',
      render: (value: string) => (
        <Space>
          <Typography.Text copyable>{value}</Typography.Text>
        </Space>
      ),
    },
    { title: '域名', dataIndex: 'domain' },
    {
      title: '类型',
      dataIndex: 'is_permanent',
      render: (permanent: boolean) =>
        permanent ? <Tag color="gold">永久</Tag> : <Tag>临时</Tag>,
    },
    {
      title: '状态',
      render: (_, row) =>
        row.expired ? <Tag color="red">已过期</Tag> : <Tag color="green">有效</Tag>,
    },
    {
      title: '过期时间',
      render: (_, row) => (row.is_permanent ? '永不过期' : formatDate(row.expires_at)),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      render: (value: string) => formatDate(value),
    },
  ];
  const inboxColumns: ColumnsType<MailMessage> = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: '发件人', dataIndex: 'mail_from' },
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
  const columnsWithActions: ColumnsType<TemporaryMailbox> = [
    ...columns,
    {
      title: '操作',
      width: 120,
      render: (_, row) => (
        <Button
          onClick={() => {
            setInboxMailbox(row);
            // 打开收件弹窗时主动刷新一次，减少用户看到旧列表的概率。
            void onChanged();
          }}
        >
          收件
        </Button>
      ),
    },
  ];

  return (
    <Card
      title="申请邮箱"
      extra={
        <Button loading={loading} onClick={() => void onChanged()}>
          刷新
        </Button>
      }
    >
      <Typography.Paragraph>
        可以自己填写邮箱名称；不填写时系统会自动生成。未选择域名时，会自动选择一个可用域名。临时邮箱到期后不再接收邮件；永久邮箱不会过期，需管理员为你开启权限。
      </Typography.Paragraph>
      <Form
        form={form}
        layout="inline"
        onFinish={create}
        className="toolbar-form"
        initialValues={{ ttl_minutes: ttlOptions[0] }}
      >
        <Form.Item name="domain">
          <Select
            allowClear
            placeholder="不选则随机域名"
            style={{ width: 220 }}
            options={selectableDomains.map((domain) => ({
              label: domain.domain,
              value: domain.domain,
            }))}
          />
        </Form.Item>
        <Form.Item
          name="local_part"
          label="邮箱名称"
          rules={[
            {
              pattern: /^[a-zA-Z0-9](?!.*\.\.)[a-zA-Z0-9._-]{1,62}[a-zA-Z0-9]$/,
              message: '请输入 3-64 位字母、数字、点、下划线或短横线',
            },
          ]}
        >
          <Input placeholder="不填则自动生成" style={{ width: 180 }} />
        </Form.Item>
        <Form.Item
          name="ttl_minutes"
          label="有效时间"
          rules={[{ required: true, message: '请选择有效时间' }]}
        >
          <Select
            placeholder="选择有效时间"
            style={{ width: 180 }}
            options={ttlOptions
              .map((minutes) => ({
                label: formatTTLMinutes(minutes),
                value: minutes,
              }))
              .concat(
                currentUser.can_lease_permanent_mailbox
                  ? [{ label: '永久', value: permanentTimeValue }]
                  : [],
              )}
          />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={submitting}>
          生成邮箱
        </Button>
      </Form>
      <Table
        rowKey="id"
        columns={columnsWithActions}
        dataSource={filteredMailboxes}
        loading={loading}
        pagination={{ pageSize: 10 }}
        title={() => (
          <Input.Search
            allowClear
            placeholder="查询邮箱地址或域名"
            value={mailboxQuery}
            onChange={(event) => setMailboxQuery(event.target.value)}
            onSearch={setMailboxQuery}
            style={{ maxWidth: 320 }}
          />
        )}
      />
      <Modal
        title="临时邮箱收件"
        open={Boolean(inboxMailbox)}
        width={860}
        footer={null}
        onCancel={() => setInboxMailbox(null)}
      >
        {inboxMailbox ? (
          <>
            <Typography.Paragraph className="mailbox-inbox-address">
              <Typography.Text code>{inboxMailbox.address}</Typography.Text>
            </Typography.Paragraph>
            <div className="mailbox-inbox-toolbar">
              <Button
                size="small"
                loading={loading}
                onClick={() => {
                  void onChanged();
                }}
              >
                刷新
              </Button>
            </div>
            <Table
              rowKey="id"
              columns={inboxColumns}
              dataSource={inboxMessages}
              loading={loading}
              pagination={{ pageSize: 5 }}
              size="small"
            />
          </>
        ) : null}
      </Modal>
    </Card>
  );
}
