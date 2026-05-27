/**
 * formatDate 将 API 时间戳渲染为表格单元格文本。
 *
 * 参数：
 * - value：ISO 时间戳字符串。
 * 返回值：本地化时间戳或兜底文本。
 * 失败条件：无。
 */
export function formatDate(value: string) {
  if (!value) {
    return '-';
  }

  return new Date(value).toLocaleString();
}

/**
 * formatTTLMinutes 将租赁分钟数格式化成适合表单和表格展示的中文文案。
 *
 * 参数：
 * - minutes：租赁分钟数。
 * 返回值：分钟、小时或天的展示文案。
 * 失败条件：无。
 */
export function formatTTLMinutes(minutes: number) {
  if (minutes % 1440 === 0) {
    return `${minutes / 1440} 天`;
  }
  if (minutes % 60 === 0) {
    return `${minutes / 60} 小时`;
  }

  return `${minutes} 分钟`;
}
