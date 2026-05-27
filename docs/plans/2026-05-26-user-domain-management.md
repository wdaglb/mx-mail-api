# 用户与域名管理方案

## 背景

当前 SMTP/MX 收件服务通过 `config.yaml` 中的 `smtp.accept_domains` 判断 `RCPT TO` 是否接收。该方式适合本地开发，但无法在运行后通过管理界面维护域名，也无法区分不同用户的域名管理边界。

本次调整将接受域名改为 Postgres 数据配置，并增加用户管理与前端管理页面。

## 已确认范围

- 后端使用 Gin API + Postgres + GORM。
- 前端使用 React + Ant Design 6.x。
- 用户认证使用本地用户 + bcrypt 密码哈希 + JWT。
- 用户角色分为：
  - `admin`：可管理全部用户和全部域名。
  - `user`：只能管理自己添加的域名。
- 默认管理员从 `config.yaml` 初始化，且仅当用户表为空时创建。
- SMTP 在 `RCPT TO` 阶段从数据库读取接受域名，不再依赖 YAML 静态域名列表。
- 本次同时交付后端 API 与前端页面。

## 后端设计

### 数据模型

- `users`
  - `id`
  - `username`
  - `password_hash`
  - `role`
  - `created_at`
  - `updated_at`
- `accepted_domains`
  - `id`
  - `domain`
  - `owner_user_id`
  - `created_at`
  - `updated_at`

域名支持精确域名和 `*.` 通配域名。`*.example.com` 只匹配子域名，不隐式匹配 `example.com` 根域名。

### 配置

`config.yaml` 增加：

```yaml
auth:
  jwt_secret: "change-me"
  token_ttl_hours: 24

admin:
  initial_user:
    username: "admin"
    password: "admin123456"
```

### API

- `POST /api/auth/login`
- `GET /api/me`
- `GET /api/users`
- `POST /api/users`
- `PUT /api/users/:id`
- `DELETE /api/users/:id`
- `GET /api/domains`
- `POST /api/domains`
- `PUT /api/domains/:id`
- `DELETE /api/domains/:id`

用户管理接口仅 `admin` 可访问。域名接口中，`admin` 可访问全部记录，`user` 只能访问 `owner_user_id` 为自己的记录。

## 前端设计

- 登录页：提交用户名和密码，保存 JWT。
- 管理主界面：
  - 顶部显示当前用户与角色。
  - 域名管理页：所有登录用户可用，普通用户仅管理自己的域名。
  - 用户管理页：仅管理员可用。

## 验证计划

- 后端：
  - `go test ./...`
  - 覆盖登录、JWT 鉴权、角色授权、域名所有权、SMTP 域名匹配。
- 前端：
  - `pnpm run build`
  - 必要时运行 `pnpm run lint`。

## 风险与后续

- JWT secret 必须在实际部署前修改，不能使用示例值。
- AutoMigrate 适合当前 MVP；生产环境如需严格审计，应后续迁移到显式 migration。
- 当前不会实现注册、邮箱内容查看、TLS、SMTP AUTH 或外发投递。
