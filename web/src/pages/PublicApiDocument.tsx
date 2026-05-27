import { Card, Typography, message } from 'antd';
import { useEffect, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import { Link } from 'react-router-dom';
import remarkGfm from 'remark-gfm';
import { errorMessage } from '../lib/errors';

/**
 * PublicApiDocument 渲染无需登录即可查看的开放接口文档。
 *
 * 参数：
 * 返回值：公开 API 文档页面。
 * 失败条件：无。
 */
export function PublicApiDocument() {
  const [markdown, setMarkdown] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadDocument = async () => {
      try {
        const response = await fetch('/docs/api.md');
        if (!response.ok) {
          throw new Error(`接口文档加载失败：${response.status}`);
        }
        setMarkdown(await response.text());
      } catch (error) {
        message.error(errorMessage(error));
      } finally {
        setLoading(false);
      }
    };
    queueMicrotask(() => void loadDocument());
  }, []);

  return (
    <div className="public-doc-page">
      <Card
        title="开放接口文档"
        className="public-doc-card"
        extra={<Link to="/">返回</Link>}
      >
        <Typography.Paragraph>
          此页面无需登录即可查看。需要查看文档原文时，可直接访问
          <Typography.Text code>/docs/api.md</Typography.Text>。
        </Typography.Paragraph>
        <div className="api-doc-markdown">
          {loading ? (
            <Typography.Text type="secondary">文档加载中...</Typography.Text>
          ) : (
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {markdown}
            </ReactMarkdown>
          )}
        </div>
      </Card>
    </div>
  );
}
