/**
 * copyTextToClipboard 将文本写入剪贴板，并提供 textarea 兜底方案。
 *
 * 参数：
 * - text：需要复制的内容。
 * 返回值：无。
 * 失败条件：现代 Clipboard API 和旧版 execCommand 兜底都失败时抛出错误。
 * 保留兜底是因为本地 HTTP 开发环境可能不会暴露 navigator.clipboard。
 */
export async function copyTextToClipboard(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }

  const textarea = document.createElement('textarea');
  textarea.value = text;
  // Note: 固定到视口外，避免复制时页面滚动或弹窗布局抖动。
  textarea.style.position = 'fixed';
  textarea.style.left = '-9999px';
  textarea.style.top = '0';
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();

  try {
    if (!document.execCommand('copy')) {
      throw new Error('浏览器未允许复制到剪贴板');
    }
  } finally {
    document.body.removeChild(textarea);
  }
}
