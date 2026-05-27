/**
 * errorMessage 将未知错误转换为可展示文本。
 *
 * 参数：
 * - error：捕获到的错误。
 * 返回值：消息字符串。
 * 失败条件：无。
 */
export function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : '操作失败';
}
