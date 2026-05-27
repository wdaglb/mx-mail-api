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
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useState } from 'react';
import type { ApiClient } from '../lib/api';
import { errorMessage } from '../lib/errors';
import { formatDate, formatTTLMinutes } from '../lib/format';
import { normalizeTemporaryMailboxTTLMinutes } from '../lib/temporaryMailbox';
import type { Role, User, UserFormValues } from '../types';

/**
 * UserPanel 渲染仅管理员可用的本地用户管理。
 *
 * 参数：
 * - api：已认证的 API 客户端。
 * - users：用户行。
 * - loading：表格加载状态。
 * - onChanged：变更后的刷新回调。
 * 返回值：用户管理面板。
 * 失败条件：变更失败会通过 Ant Design message 展示。
 */
export function UserPanel({
  api,
  users,
  loading,
  onChanged,
}: {
  api: ApiClient;
  users: User[];
  loading: boolean;
  onChanged: () => Promise<void>;
}) {
  const [form] = Form.useForm<UserFormValues>();
  const [editing, setEditing] = useState<User | null>(null);
  const [modalOpen, setModalOpen] = useState(false);

  const openCreateModal = () => {
    setEditing(null);
    form.setFieldsValue({
      username: '',
      password: '',
      role: 'user',
      temporary_mailbox_ttl_minutes: [30],
      can_lease_permanent_mailbox: false,
      openai_qos_rpm: 60,
    });
    setModalOpen(true);
  };

  const openEditModal = (row: User) => {
    setEditing(row);
    form.setFieldsValue({
      username: row.username,
      role: row.role,
      password: '',
      temporary_mailbox_ttl_minutes: normalizeTemporaryMailboxTTLMinutes(
        row.temporary_mailbox_ttl_minutes,
      ),
      can_lease_permanent_mailbox: row.can_lease_permanent_mailbox,
      openai_qos_rpm: row.openai_qos_rpm || 60,
    });
    setModalOpen(true);
  };

  const closeModal = () => {
    setModalOpen(false);
    setEditing(null);
    form.resetFields();
  };

  const save = async (values: UserFormValues) => {
    const payload = {
      ...values,
      // 后端也会做范围校验；前端先去重排序，减少管理员误填导致的往返失败。
      temporary_mailbox_ttl_minutes: normalizeTemporaryMailboxTTLMinutes(
        values.temporary_mailbox_ttl_minutes,
      ),
    };

    try {
      if (editing) {
        await api.put(`/api/users/${editing.id}`, payload);
        message.success('用户已更新');
      } else {
        await api.post('/api/users', payload);
        message.success('用户已创建');
      }
      form.resetFields();
      setEditing(null);
      setModalOpen(false);
      await onChanged();
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const columns: ColumnsType<User> = [
    { title: '用户名', dataIndex: 'username' },
    {
      title: '角色',
      dataIndex: 'role',
      render: (role: Role) => <Tag>{role}</Tag>,
    },
    {
      title: '邮箱有效时间',
      dataIndex: 'temporary_mailbox_ttl_minutes',
      render: (values: number[]) => (
        <Space wrap>
          {normalizeTemporaryMailboxTTLMinutes(values).map((value) => (
            <Tag key={value}>{formatTTLMinutes(value)}</Tag>
          ))}
        </Space>
      ),
    },
    {
      title: '永久邮箱',
      dataIndex: 'can_lease_permanent_mailbox',
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'green' : 'default'}>
          {enabled ? '可申请' : '不可申请'}
        </Tag>
      ),
    },
    {
      title: '接口调用额度',
      dataIndex: 'openai_qos_rpm',
      render: (value: number) => <Tag>{value || 60} 次/分钟</Tag>,
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
          <Button onClick={() => openEditModal(row)}>编辑</Button>
          <Popconfirm
            title="确认删除该用户？"
            onConfirm={async () => {
              try {
                await api.delete(`/api/users/${row.id}`);
                message.success('用户已删除');
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
      title="用户"
      extra={
        <Button type="primary" onClick={openCreateModal}>
          新增用户
        </Button>
      }
    >
      <UserEditorModal
        editing={editing}
        form={form}
        open={modalOpen}
        onCancel={closeModal}
        onSave={save}
      />
      <Table
        rowKey="id"
        columns={columns}
        dataSource={users}
        loading={loading}
        pagination={false}
      />
    </Card>
  );
}
/**
 * UserEditorModal 渲染用户新增和编辑弹窗。
 *
 * 参数：
 * - editing：当前编辑用户；为空表示新增。
 * - form：父组件持有的表单实例，方便打开弹窗前写入默认值。
 * - open：弹窗是否展示。
 * - onCancel：取消回调。
 * - onSave：表单校验通过后的保存回调。
 * 返回值：用户编辑弹窗。
 * 失败条件：表单校验失败时不会触发保存。
 */
function UserEditorModal({
  editing,
  form,
  open,
  onCancel,
  onSave,
}: {
  editing: User | null;
  form: ReturnType<typeof Form.useForm<UserFormValues>>[0];
  open: boolean;
  onCancel: () => void;
  onSave: (values: UserFormValues) => Promise<void>;
}) {
  return (
    <Modal
      title={editing ? '编辑用户' : '新增用户'}
      open={open}
      okText={editing ? '保存用户' : '新增用户'}
      cancelText="取消"
      destroyOnHidden
      onCancel={onCancel}
      onOk={() => form.submit()}
    >
      <Form
        form={form}
        layout="vertical"
        onFinish={onSave}
        initialValues={{
          role: 'user',
          temporary_mailbox_ttl_minutes: [30],
          can_lease_permanent_mailbox: false,
          openai_qos_rpm: 60,
        }}
      >
        <Form.Item
          name="username"
          label="用户名"
          rules={[{ required: true, message: '请输入用户名' }]}
        >
          <Input placeholder="用户名" />
        </Form.Item>
        <Form.Item
          name="password"
          label="密码"
          rules={editing ? [] : [{ required: true, message: '请输入密码' }]}
          tooltip="编辑时留空表示不修改密码"
        >
          <Input.Password placeholder={editing ? '留空不修改密码' : '密码'} />
        </Form.Item>
        <Form.Item
          name="role"
          label="角色"
          rules={[{ required: true, message: '请选择角色' }]}
        >
          <Select
            options={[
              { label: 'admin', value: 'admin' },
              { label: 'user', value: 'user' },
            ]}
          />
        </Form.Item>
        <Form.Item
          name="can_lease_permanent_mailbox"
          label="允许申请永久邮箱"
          valuePropName="checked"
          tooltip="管理员账号始终允许申请永久邮箱；普通用户按此配置控制。"
        >
          <Switch />
        </Form.Item>
        <Form.Item
          name="openai_qos_rpm"
          label="接口调用额度"
          rules={[
            { required: true, message: '请输入每分钟最多可使用次数' },
            {
              type: 'number',
              min: 1,
              max: 100000,
              transform: (value) => Number(value),
              message: '每分钟最多可使用次数必须在 1 到 100000 之间',
            },
          ]}
          tooltip="控制该用户每分钟最多可以调用接口的次数。"
        >
          <Input type="number" min={1} max={100000} placeholder="例如 60" />
        </Form.Item>
        <Form.Item
          name="temporary_mailbox_ttl_minutes"
          label="邮箱有效时间"
          rules={[
            {
              required: true,
              message: '请输入至少一个租赁时间',
            },
            {
              validator: async (_, values?: number[]) => {
                const normalized =
                  normalizeTemporaryMailboxTTLMinutes(values);
                if (normalized.length === 0) {
                  throw new Error('请输入至少一个租赁时间');
                }
              },
            },
          ]}
          tooltip="可配置多个有效时间，最短 1 分钟，最长 7 天。"
        >
          <Select
            mode="tags"
            tokenSeparators={[',', '，', ' ']}
            placeholder="例如 30, 60, 1440"
            options={[
              { label: '30 分钟', value: 30 },
              { label: '1 小时', value: 60 },
              { label: '6 小时', value: 360 },
              { label: '1 天', value: 1440 },
            ]}
          />
        </Form.Item>
      </Form>
    </Modal>
  );
}

type ApiClient = ReturnType<typeof createApiClient>;
