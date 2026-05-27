import { Avatar, Button, Dropdown, Space, Typography } from 'antd';
import type { MenuProps } from 'antd';
import type { User } from '../types';

/**
 * userAvatarText 生成用户头像展示字符。
 *
 * 参数：
 * - username：当前登录用户名。
 * 返回值：用户名首字符的大写形式；用户名为空时使用问号兜底。
 * 失败条件：无。
 */
function userAvatarText(username: string) {
  const trimmed = username.trim();
  if (!trimmed) {
    return '?';
  }

  return trimmed.slice(0, 1).toUpperCase();
}

/**
 * UserProfile 渲染 Header 右上角用户入口。
 *
 * 参数：
 * - user：当前登录用户。
 * - onOpenApiKey：打开 API Key 弹窗。
 * - onLogout：退出登录。
 * 返回值：用户信息按钮和下拉菜单。
 * 失败条件：无；实际 API Key 操作由父组件弹窗处理。
 */
export function UserProfile({
  user,
  onOpenApiKey,
  onLogout,
}: {
  user: User;
  onOpenApiKey: () => void;
  onLogout: () => void;
}) {
  const isAdmin = user.role === 'admin';
  const items: MenuProps['items'] = [
    {
      key: 'profile',
      disabled: true,
      label: `角色：${isAdmin ? '管理员' : '普通用户'}`,
    },
    {
      type: 'divider',
    },
    {
      key: 'docs',
      label: (
        <a href="/docs" target="_blank" rel="noreferrer">
          接口文档
        </a>
      ),
    },
    {
      key: 'api-key',
      label: '访问密钥',
      onClick: onOpenApiKey,
    },
    {
      type: 'divider',
    },
    {
      key: 'logout',
      danger: true,
      label: '退出登录',
      onClick: onLogout,
    },
  ];

  return (
    <Dropdown menu={{ items }} trigger={['click']} placement="bottomRight">
      <Button className="user-profile-button">
        <Space size={8}>
          <Avatar className="user-profile-avatar" size={24}>
            {userAvatarText(user.username)}
          </Avatar>
          <Typography.Text strong>{user.username}</Typography.Text>
        </Space>
      </Button>
    </Dropdown>
  );
}
