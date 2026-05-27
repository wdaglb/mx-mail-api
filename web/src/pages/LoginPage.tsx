import { Button, Card, Form, Input, message } from 'antd';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { createApiClient } from '../lib/api';
import { errorMessage } from '../lib/errors';
import type { User } from '../types';

/**
 * LoginPage 渲染用户名密码登录页。
 *
 * 参数：
 * - onLogin：接收 token 和已认证用户的回调。
 * 返回值：登录页组件。
 * 失败条件：API 错误会以页面消息展示。
 */
export function LoginPage({
  onLogin,
}: {
  onLogin: (token: string, user: User) => void;
}) {
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const submit = async (values: { username: string; password: string }) => {
    setLoading(true);
    try {
      const data = await createApiClient('').post<{
        token: string;
        user: User;
      }>('/api/auth/login', values);
      onLogin(data.token, data.user);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-page">
      <Card title="登录 Mx Mail Api" className="login-card">
        <Form layout="vertical" onFinish={submit}>
          <Form.Item
            name="username"
            label="用户名"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input autoComplete="username" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={loading} block>
            登录
          </Button>
        </Form>
        <Button type="link" block onClick={() => navigate('/docs')}>
          查看开放接口文档
        </Button>
      </Card>
    </div>
  );
}
