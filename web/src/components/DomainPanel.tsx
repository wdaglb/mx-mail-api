import {
  Button,
  Card,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useCallback, useEffect, useState } from 'react';
import type { ApiClient } from '../lib/api';
import { errorMessage } from '../lib/errors';
import { formatDate } from '../lib/format';
import type {
  Domain,
  DomainFormValues,
  DomainVerification,
  PublicConfig,
  User,
} from '../types';

/**
 * normalizeDomainText 归一化前端展示和拼接用的域名文本。
 *
 * 参数：
 * - value：用户输入的域名或记录名。
 * 返回值：去除首尾空白、尾部点并转小写后的文本。
 * 失败条件：无；格式合法性仍由后端统一校验。
 */
function normalizeDomainText(value: string) {
  return value.trim().toLowerCase().replace(/\.$/, '');
}

/**
 * buildVerificationRecordName 根据随机 TXT 前缀和当前域名生成完整记录名。
 *
 * 参数：
 * - recordName：后端生成的随机记录名前缀，或之前补齐过域名的完整记录名。
 * - domain：当前表单中的待验证域名。
 * 返回值：可展示给用户配置 DNS 的 TXT 记录名。
 * 失败条件：无；用户最终提交的记录名仍由后端再次校验。
 */
function buildVerificationRecordName(recordName: string, domain: string) {
  const normalizedName = normalizeDomainText(recordName);
  const normalizedDomain = normalizeDomainText(domain);
  if (!normalizedName || !normalizedDomain) {
    return normalizedName;
  }

  const suffix = `.${normalizedDomain}`;
  if (normalizedName.endsWith(suffix)) {
    return normalizedName;
  }

  // 记录名由后端生成随机字母数字前缀；用户切换域名时只替换域名部分，不重新生成验证值。
  const label = normalizedName.split('.')[0];
  return label ? `${label}${suffix}` : normalizedName;
}

/**
 * DomainPanel 渲染接受域名管理面板。
 *
 * 参数：
 * - api：已认证的 API 客户端。
 * - domains：可见域名行。
 * - users：管理员选择所有者时可用的用户列表。
 * - currentUser：已认证用户。
 * - publicConfig：后端 yaml 中可公开展示的运行配置。
 * - loading：表格加载状态。
 * - onChanged：变更后的刷新回调。
 * 返回值：域名管理面板。
 * 失败条件：变更失败会通过 Ant Design message 展示。
 */
export function DomainPanel({
  api,
  domains,
  users,
  currentUser,
  publicConfig,
  loading,
  onChanged,
}: {
  api: ApiClient;
  domains: Domain[];
  users: User[];
  currentUser: User;
  publicConfig: PublicConfig | null;
  loading: boolean;
  onChanged: () => Promise<void>;
}) {
  const [form] = Form.useForm<DomainFormValues>();
  const [editing, setEditing] = useState<Domain | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [verification, setVerification] = useState<DomainVerification | null>(
    null,
  );
  const isAdmin = currentUser.role === 'admin';
  const smtpHostname = publicConfig?.smtp_hostname || 'mail.example.com';
  const watchedDomain = Form.useWatch('domain', form);

  const save = async (values: DomainFormValues) => {
    setSubmitting(true);
    try {
      const payload = {
        ...values,
        // 前端用 0 代表“所有人”，提交前转成 null，避免后端误解为真实用户 ID。
        owner_user_id: values.owner_user_id === 0 ? null : values.owner_user_id,
        disabled: Boolean(values.disabled),
      };
      if (editing) {
        await api.put(`/api/domains/${editing.id}`, payload);
        message.success('域名已更新');
      } else {
        await api.post('/api/domains', payload);
        message.success('域名已添加');
      }
      form.resetFields();
      setEditing(null);
      setModalOpen(false);
      setVerification(null);
      await onChanged();
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSubmitting(false);
    }
  };

  const generateVerification = useCallback(async (domain: string) => {
    try {
      const data = await api.post<{ item: DomainVerification }>(
        '/api/domains/verification',
        { domain },
      );
      const currentDomain = form.getFieldValue('domain') || domain;
      const next = {
        ...data.item,
        // 后端返回随机记录名前缀；当前已输入域名时前端补齐为完整 TXT 记录名，便于直接复制解析。
        name: buildVerificationRecordName(data.item.name || '', currentDomain),
      };
      setVerification(next);
      form.setFieldsValue({
        verification_name: next.name,
        verification_value: next.value,
      });
    } catch (error) {
      message.error(errorMessage(error));
    }
  }, [api, form]);

  useEffect(() => {
    if (!modalOpen || editing) {
      return;
    }

    const timer = window.setTimeout(() => {
      const domain = watchedDomain || '';
      if (verification?.value) {
        const name = buildVerificationRecordName(verification.name, domain);
        if (name !== verification.name) {
          setVerification({ ...verification, name });
          form.setFieldsValue({ verification_name: name });
        }
        return;
      }

      void generateVerification(domain);
    }, 300);

    // 用户继续输入域名时取消上一轮同步，避免给旧域名展示 TXT 记录名。
    return () => window.clearTimeout(timer);
  }, [editing, form, generateVerification, modalOpen, verification, watchedDomain]);

  const openCreateModal = () => {
    setEditing(null);
    form.resetFields();
    setVerification(null);
    // 管理员新增时默认选择“所有人”，普通用户不展示该字段，后端会按当前用户归属处理。
    form.setFieldsValue({
      domain: '',
      owner_user_id: isAdmin ? 0 : undefined,
      disabled: false,
      verification_name: '',
      verification_value: '',
    });
    setModalOpen(true);
    // 打开新增弹窗后立即生成随机 TXT 记录名前缀和值，域名输入后再自动补齐完整记录名。
    void generateVerification('');
  };

  const openEditModal = (row: Domain) => {
    setEditing(row);
    setVerification(null);
    form.setFieldsValue({
      domain: row.domain,
      owner_user_id: row.owner_user_id ?? 0,
      disabled: row.disabled,
    });
    setModalOpen(true);
  };

  const columns: ColumnsType<Domain> = [
    { title: '域名', dataIndex: 'domain' },
    {
      title: '所有者',
      dataIndex: 'owner_name',
      render: (_, row) => row.owner_name || row.owner_user_id || '所有人',
    },
    {
      title: '状态',
      dataIndex: 'disabled',
      render: (disabled: boolean) =>
        disabled ? <Tag color="red">已禁用</Tag> : <Tag color="green">启用中</Tag>,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      render: (value: string) => formatDate(value),
    },
    {
      title: '操作',
      width: 180,
      render: (_, row) => (
        <Space>
          <Button
            onClick={() => {
              openEditModal(row);
            }}
          >
            编辑
          </Button>
          <Popconfirm
            title="确认删除该域名？"
            onConfirm={async () => {
              try {
                await api.delete(`/api/domains/${row.id}`);
                message.success('域名已删除');
                await onChanged();
              } catch (error) {
                message.error(errorMessage(error));
              }
            }}
          >
            <Button danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card
      title="接受域名"
      extra={
        <Space>
          <Button loading={loading} onClick={() => void onChanged()}>
            刷新
          </Button>
          <Button type="primary" onClick={openCreateModal}>
            新增域名
          </Button>
        </Space>
      }
    >
      <Table
        rowKey="id"
        columns={columns}
        dataSource={domains}
        loading={loading}
        pagination={false}
      />
      <Modal
        title={editing ? '编辑域名' : '新增域名'}
        open={modalOpen}
        okText={editing ? '保存域名' : '新增域名'}
        confirmLoading={submitting}
        cancelText="取消"
        maskClosable={!submitting}
        onCancel={() => {
          if (submitting) {
            return;
          }
          setModalOpen(false);
          setEditing(null);
          setVerification(null);
          form.resetFields();
        }}
        onOk={() => void form.submit()}
      >
        <Form form={form} layout="vertical" onFinish={save}>
          <div className="dns-guide">
              <Typography.Title level={5}>域名解析说明</Typography.Title>
              <Typography.Paragraph>
                请先在域名服务商后台添加以下解析记录，确保邮件可以投递到本系统。
              </Typography.Paragraph>
              <pre>{`@ MX ${smtpHostname}`}</pre>
          </div>
          {!editing ? (
            <div className="dns-guide">
              <Typography.Title level={5}>域名所有权验证</Typography.Title>
              <Typography.Paragraph>
                为确认该域名属于你，请按下方记录名和值添加一条验证解析。解析生效后再提交保存。
              </Typography.Paragraph>
            </div>
          ) : null}
          <Form.Item
            name="domain"
            label="域名"
            rules={[{ required: true, message: '请输入域名' }]}
          >
            <Input placeholder="example.com" />
          </Form.Item>
          {!editing ? (
            <>
              <Form.Item
                name="verification_name"
                label="验证记录名"
                rules={[{ required: true, message: '请等待系统生成验证信息' }]}
              >
                <Input readOnly placeholder="a1b2c3d4e5f6.example.com" />
              </Form.Item>
              <Form.Item
                name="verification_value"
                label="验证记录值"
                rules={[{ required: true, message: '请等待系统生成验证信息' }]}
              >
                <Input.TextArea readOnly autoSize />
              </Form.Item>
            </>
          ) : null}
          {isAdmin ? (
            <Form.Item name="owner_user_id" label="所有者">
              <Select
                placeholder="所有者"
                options={users
                  .map((user) => ({
                    label: user.username,
                    value: user.id,
                  }))
                  .concat([{ label: '所有人', value: 0 }])}
              />
            </Form.Item>
          ) : null}
          <Form.Item
            name="disabled"
            label="是否禁用"
            valuePropName="checked"
            tooltip="禁用后，该域名不会出现在邮箱申请列表中，也不会再接收新邮件。"
          >
            <Switch checkedChildren="禁用" unCheckedChildren="启用" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}
