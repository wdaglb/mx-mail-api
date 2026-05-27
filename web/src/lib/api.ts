import type { ApiError } from '../types';

export type ApiClient = ReturnType<typeof createApiClient>;

/**
 * createApiClient 基于 fetch 创建轻量类型化 JSON 客户端。
 *
 * 参数：
 * - token：可选 JWT 访问令牌。
 * 返回值：JSON API 辅助方法。
 * 失败条件：网络失败、JSON 解析失败，或 API 返回非 2xx 响应时 reject。
 */
export function createApiClient(token: string) {
  const request = async <T,>(
    method: string,
    path: string,
    payload?: unknown,
  ): Promise<T> => {
    const response = await fetch(path, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: payload ? JSON.stringify(payload) : undefined,
    });

    if (!response.ok) {
      const body = (await response.json().catch(() => ({}))) as ApiError;
      throw new Error(body.error?.message || `请求失败：${response.status}`);
    }

    if (response.status === 204) {
      return undefined as T;
    }

    return (await response.json()) as T;
  };

  return {
    get: <T,>(path: string) => request<T>('GET', path),
    post: <T,>(path: string, payload: unknown) =>
      request<T>('POST', path, payload),
    put: <T,>(path: string, payload: unknown) =>
      request<T>('PUT', path, payload),
    delete: <T,>(path: string) => request<T>('DELETE', path),
  };
}
