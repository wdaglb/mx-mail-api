import { Button, Input, Modal, Typography, message } from 'antd';
import type { ApiClient } from '../lib/api';
import { copyTextToClipboard } from '../lib/clipboard';
import { errorMessage } from '../lib/errors';
import type { ApiKeyResult, User } from '../types';

/**
 * ApiKeyModal 渲染 API Key 管理弹窗。
 *
 * 参数：
 * - api：已认证的 API 客户端。
 * - currentUser：当前认证用户，用于决定按钮显示生成或重置。
 * - loading：生成或重置请求的加载状态。
 * - open：弹窗是否打开。
 * - token：本次生成后仅展示一次的明文 API Key。
 * - onClose：关闭弹窗回调。
 * - onGenerated：生成或重置成功后的结果回调。
 * - onLoadingChange：同步请求加载状态的回调。
 * 返回值：API Key 管理弹窗。
 * 失败条件：生成或重置失败会通过 Ant Design message 展示。
 */
export function ApiKeyModal({
  api,
  currentUser,
  loading,
  open,
  token,
  onClose,
  onGenerated,
  onLoadingChange,
}: {
  api: ApiClient;
  currentUser: User;
  loading: boolean;
  open: boolean;
  token: string;
  onClose: () => void;
  onGenerated: (result: ApiKeyResult) => void;
  onLoadingChange: (loading: boolean) => void;
}) {
  const actionText = currentUser.has_api_key ? '重置访问密钥' : '生成访问密钥';

  const generate = async () => {
    onLoadingChange(true);
    try {
      const data = await resetApiKey(api);
      onGenerated(data.item);
      message.success(`${actionText}成功`);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      onLoadingChange(false);
    }
  };

  return (
    <Modal
      footer={null}
      onCancel={onClose}
      open={open}
      title="访问密钥管理"
      width={680}
    >
      <Typography.Paragraph>
        密钥可用于在其他程序中访问你的邮箱服务。为保护安全，密钥内容只会在生成或重置后展示一次。
      </Typography.Paragraph>
      <Button loading={loading} onClick={() => void generate()} type="primary">
        {actionText}
      </Button>
      {token ? (
        <div className="api-key-token-block">
          <Typography.Paragraph>
            请立即复制保存。关闭后无法再次查看，只能重新生成。
          </Typography.Paragraph>
          {/* API Key 明文只在本次响应中返回，复制操作必须放在一次性展示区域内。 */}
          <Input.TextArea value={token} autoSize readOnly />
          <Button
            className="api-key-copy-button"
            onClick={() => void copyGeneratedApiKey(token)}
          >
            复制访问密钥
          </Button>
        </div>
      ) : null}
    </Modal>
  );
}

/**
 * resetApiKey 请求服务端重置当前用户的 API Key。
 *
 * 参数：
 * - api：已认证的 API 客户端。
 * 返回值：包含一次性明文 token 和更新后用户资料的响应。
 * 失败条件：API 请求失败时抛出错误，由调用方决定如何展示。
 */
async function resetApiKey(api: ApiClient) {
  return api.post<{ item: ApiKeyResult }>('/api/me/api-key', {});
}

/**
 * copyGeneratedApiKey 将一次性 API Key 复制到系统剪贴板。
 *
 * 参数：
 * - token：服务端在本次重置操作中返回的明文 API Key。
 * 返回值：无。
 * 失败条件：浏览器拒绝剪贴板访问，或没有兼容复制兜底方案时展示错误。
 */
async function copyGeneratedApiKey(token: string) {
  try {
    await copyTextToClipboard(token);
    message.success('访问密钥已复制');
  } catch (error) {
    message.error(errorMessage(error));
  }
}
