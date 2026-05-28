# Mx Mail API 开放接口文档

## 认证方式

开放接口使用用户自己的 API Key 认证。API Key 可以在登录后台后通过右上角“API Key”按钮生成或重置。

请求时任选一种方式传入：

```http
X-API-Key: <your_api_key>
```

或：

```http
Authorization: Bearer <your_api_key>
```

## 租赁临时邮箱

租赁临时邮箱前，后台需要先添加并验证可用域名。新增域名时系统会生成 TXT
记录值，只有 DNS TXT 解析匹配后才会保存域名，避免未授权使用他人域名。

```http
POST /openapi/temporary-mailboxes
Content-Type: application/json
X-API-Key: <your_api_key>
```

请求体：

```json
{
  "domain": "example.com",
  "ttl_minutes": 30,
  "permanent": false
}
```

字段说明：

- `domain`：可选。指定当前用户可用域名；不传时服务端会从当前用户可用域名中随机选择。
- `ttl_minutes`：可选。必须是当前用户允许的租赁时长；不传时使用当前用户配置里的第一个租赁时长。
- `permanent`：可选。传 `true` 时申请永久邮箱；需要管理员先为当前用户开启永久邮箱权限，此时可不传 `ttl_minutes`。

行为说明：

- 服务端自动生成 8 位随机邮箱前缀，不包含固定业务前缀。
- 已禁用的域名不会被用于申请邮箱，也不会在未指定域名时被随机选中。
- 永久邮箱不会过期；普通邮箱到期后不再接收邮件。

成功响应：

```json
{
  "item": {
    "id": 1,
    "address": "abcd1234@example.com",
    "local_part": "abcd1234",
    "domain": "example.com",
    "owner_user_id": 2,
    "expires_at": "2026-05-27T12:30:00+08:00",
    "is_permanent": false,
    "created_at": "2026-05-27T12:00:00+08:00",
    "expired": false,
    "ttl_minutes": 30
  }
}
```

常见错误：

- `401 unauthorized`：API Key 缺失或无效。
- `400 invalid_domain`：域名不属于当前用户可用范围。
- `400 invalid_ttl`：租赁时间不在当前用户允许范围内。
- `403 forbidden`：当前用户没有申请永久邮箱的权限。

## 获取临时邮箱最新邮件

```http
GET /openapi/temporary-mailboxes/latest-message?address=abcd1234@example.com
X-API-Key: <your_api_key>
```

查询参数：

- `address`：必填。通过租赁接口获得的完整临时邮箱地址。

成功响应：

```json
{
  "item": {
    "from": "sender@example.com",
    "subject": "邮件主题",
    "body": "邮件正文",
    "created_at": "2026-05-27T12:01:00+08:00"
  }
}
```

行为说明：

- 只查询发给该临时邮箱地址的最新一封已入库邮件。
- 只允许读取当前 API Key 用户自己租赁的临时邮箱。
- 没有邮件时立即返回 `404`，不会等待新邮件。

常见错误：

- `400 invalid_request`：缺少 `address` 参数。
- `401 unauthorized`：API Key 缺失或无效。
- `403 forbidden`：尝试读取其他用户租赁的临时邮箱。
- `404 not_found`：临时邮箱不存在，或该临时邮箱还没有收到邮件。
