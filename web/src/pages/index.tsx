import { Layout, Menu, Typography, message } from 'antd';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { ApiKeyModal } from '../components/ApiKeyModal';
import { DomainPanel } from '../components/DomainPanel';
import { MessagePanel } from '../components/MessagePanel';
import { TemporaryMailboxPanel } from '../components/TemporaryMailboxPanel';
import { UserPanel } from '../components/UserPanel';
import { UserProfile } from '../components/UserProfile';
import { createApiClient } from '../lib/api';
import { errorMessage } from '../lib/errors';
import type {
  Domain,
  MailMessage,
  PublicConfig,
  TemporaryMailbox,
  User,
} from '../types';
import { LoginPage } from './LoginPage';

const { Header, Sider, Content } = Layout;
const tokenKey = 'mx-mail-api-token';

const defaultMenuKey = 'temporary-mailbox';

/**
 * HomePage 渲染需要登录的后台首页。
 *
 * 参数：无。
 * 返回值：登录页或已认证后台页面。
 * 失败条件：API 失败会通过 Ant Design message 展示。
 */
export function HomePage() {
  const [token, setToken] = useState(
    () => localStorage.getItem(tokenKey) || '',
  );
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [users, setUsers] = useState<User[]>([]);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [mailMessages, setMailMessages] = useState<MailMessage[]>([]);
  const [temporaryMailboxes, setTemporaryMailboxes] = useState<
    TemporaryMailbox[]
  >([]);
  const [publicConfig, setPublicConfig] = useState<PublicConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [bootstrapped, setBootstrapped] = useState(!token);
  const [apiKeyModalOpen, setApiKeyModalOpen] = useState(false);
  const [apiKeyToken, setApiKeyToken] = useState('');
  const [apiKeyLoading, setApiKeyLoading] = useState(false);
  const [selectedMenuKey, setSelectedMenuKey] = useState(defaultMenuKey);

  const isAdmin = currentUser?.role === 'admin';
  const api = useMemo(() => createApiClient(token), [token]);

  const logout = useCallback(() => {
    localStorage.removeItem(tokenKey);
    setToken('');
    setCurrentUser(null);
    setUsers([]);
    setDomains([]);
    setMailMessages([]);
    setTemporaryMailboxes([]);
  }, []);

  const loadMe = useCallback(async () => {
    if (!token) {
      setBootstrapped(true);
      return;
    }

    try {
      const data = await api.get<{ user: User }>('/api/me');
      setCurrentUser(data.user);
    } catch (error) {
      message.error(errorMessage(error));
      logout();
    } finally {
      setBootstrapped(true);
    }
  }, [api, logout, token]);

  const loadDomains = useCallback(async () => {
    if (!token) {
      return;
    }

    const data = await api.get<{ items: Domain[] }>('/api/domains');
    setDomains(data.items);
  }, [api, token]);

  const loadUsers = useCallback(async () => {
    if (!token || !isAdmin) {
      return;
    }

    const data = await api.get<{ items: User[] }>('/api/users');
    setUsers(data.items);
  }, [api, isAdmin, token]);

  const loadMailMessages = useCallback(async () => {
    if (!token) {
      return;
    }

    const data = await api.get<{ items: MailMessage[] }>('/api/messages');
    setMailMessages(data.items);
  }, [api, token]);

  const loadTemporaryMailboxes = useCallback(async () => {
    if (!token) {
      return;
    }

    const data = await api.get<{ items: TemporaryMailbox[] }>(
      '/api/temporary-mailboxes',
    );
    setTemporaryMailboxes(data.items);
  }, [api, token]);

  const loadPublicConfig = useCallback(async () => {
    const data = await createApiClient('').get<{ item: PublicConfig }>(
      '/api/public-config',
    );
    setPublicConfig(data.item);
  }, []);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      await Promise.all([
        loadDomains(),
        loadUsers(),
        loadMailMessages(),
        loadTemporaryMailboxes(),
        loadPublicConfig(),
      ]);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [
    loadDomains,
    loadMailMessages,
    loadPublicConfig,
    loadTemporaryMailboxes,
    loadUsers,
  ]);

  useEffect(() => {
    queueMicrotask(() => void loadMe());
  }, [loadMe]);

  useEffect(() => {
    if (currentUser) {
      queueMicrotask(() => void refresh());
    }
  }, [currentUser, refresh]);

  if (!bootstrapped) {
    return <div className="boot-loading">加载中...</div>;
  }

  if (!token || !currentUser) {
    return (
      <LoginPage
        onLogin={(nextToken, user) => {
          localStorage.setItem(tokenKey, nextToken);
          setToken(nextToken);
          setCurrentUser(user);
        }}
      />
    );
  }

  const menuItems = [
    { key: 'temporary-mailbox', label: '申请邮箱' },
    { key: 'messages', label: '收件记录' },
    { key: 'domains', label: '域名管理' },
    ...(isAdmin ? [{ key: 'users', label: '用户管理' }] : []),
  ];
  const activeMenuKey =
    selectedMenuKey === 'users' && !isAdmin ? defaultMenuKey : selectedMenuKey;

  const pageContent = (() => {
    switch (activeMenuKey) {
      case 'messages':
        return (
          <MessagePanel
            api={api}
            messages={mailMessages}
            loading={loading}
            onRefresh={refresh}
          />
        );
      case 'domains':
        return (
          <DomainPanel
            api={api}
            domains={domains}
            users={users}
            currentUser={currentUser}
            publicConfig={publicConfig}
            loading={loading}
            onChanged={refresh}
          />
        );
      case 'users':
        if (isAdmin) {
          return (
            <UserPanel
              api={api}
              users={users}
              loading={loading}
              onChanged={refresh}
            />
          );
        }

        // 权限变化后如果仍停留在管理员菜单，回退到默认页，避免渲染无权限内容。
        return null;
      case 'temporary-mailbox':
      default:
        return (
          <TemporaryMailboxPanel
            api={api}
            currentUser={currentUser}
            domains={domains}
            loading={loading}
            mailboxes={temporaryMailboxes}
            messages={mailMessages}
            onChanged={refresh}
          />
        );
    }
  })();

  return (
    <Layout className="app-shell">
      <Header className="app-header">
        <div>
          <Typography.Title level={4} className="app-title">
            Mx Mail Api
          </Typography.Title>
          <Typography.Text className="app-subtitle">
            邮箱与域名管理
          </Typography.Text>
        </div>
        <UserProfile
          user={currentUser}
          onOpenApiKey={() => {
            setApiKeyToken('');
            setApiKeyModalOpen(true);
          }}
          onLogout={logout}
        />
      </Header>
      <ApiKeyModal
        api={api}
        currentUser={currentUser}
        loading={apiKeyLoading}
        open={apiKeyModalOpen}
        token={apiKeyToken}
        onClose={() => setApiKeyModalOpen(false)}
        onGenerated={(result) => {
          setCurrentUser(result.user);
          setApiKeyToken(result.token);
        }}
        onLoadingChange={setApiKeyLoading}
      />
      <Layout className="app-body">
        <Sider
          className="app-sider"
          breakpoint="lg"
          collapsedWidth={0}
          width={220}
        >
          <Menu
            mode="inline"
            selectedKeys={[activeMenuKey]}
            items={menuItems}
            onClick={({ key }) => setSelectedMenuKey(key)}
          />
        </Sider>
        <Content className="app-content">{pageContent}</Content>
      </Layout>
    </Layout>
  );
}
