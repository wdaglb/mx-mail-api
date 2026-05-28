/**
 * normalizeTemporaryMailboxTTLMinutes 归一化临时邮箱租赁分钟数。
 *
 * 参数：
 * - values：后端返回或表单输入的分钟数，tags 模式下可能混入字符串。
 * 返回值：1 到 10080 分钟范围内去重、升序后的分钟数；空值按后端兼容规则显示为 30。
 * 失败条件：无；非法值会被过滤，最终仍为空时回退到默认 30 分钟。
 */
export function normalizeTemporaryMailboxTTLMinutes(
  values?: Array<number | string>,
) {
  const parsed = (values || [])
    .map((value) => Number(value))
    .filter(
      (value) => Number.isInteger(value) && value >= 1 && value <= 10080,
    );
  const unique = Array.from(new Set(parsed)).sort((left, right) => left - right);
  return unique.length > 0 ? unique : [30];
}

/**
 * isTemporaryMailboxSelectableDomain 判断域名是否能作为临时邮箱申请域名展示。
 *
 * 参数：
 * - domain：后端返回的域名规则。
 * - disabled：后端返回的禁用状态。
 * 返回值：未禁用且不包含已废弃 "*" 通配写法的域名返回 true。
 * 失败条件：无。
 */
export function isTemporaryMailboxSelectableDomain(
  domain: string,
  disabled?: boolean,
) {
  return Boolean(domain) && !disabled && !domain.includes('*');
}
