# Mx Mail API

Mx Mail API 是一个自托管的邮件接收和临时邮箱服务。它提供 SMTP 收件能力、后台管理界面和开放接口，适合用来搭建可通过 API 申请邮箱、读取最新邮件的邮件接收服务。

## 已实现能力

- SMTP/MX 收件服务：接收发往已申请邮箱的邮件，并保存原始邮件内容。
- 邮件正文解析：查看邮件时解析 MIME、编码和 HTML 正文，前端按原始 HTML 展示。
- 申请邮箱：用户可以申请临时邮箱，也可以在管理员授权后申请永久邮箱。
- 域名管理：支持添加收件域名，新增域名前需要通过 DNS TXT 记录验证所有权。
- 用户管理：管理员可以创建用户、配置用户可选邮箱有效时间、永久邮箱权限和接口调用额度。
- API Key：每个用户可以生成自己的 API Key，用于调用开放接口。
- 开放接口：支持通过接口申请邮箱、获取指定邮箱的最新邮件。
- 后台界面：内置 React + Ant Design 管理页面，包含申请邮箱、收件记录、域名管理、用户管理和接口文档。
- 一键部署：提供 Dockerfile 和 docker-compose.yaml，可同时启动应用服务和 Postgres。

## 技术栈

- 后端：Go、Gin、GORM、Postgres
- 前端：React、Ant Design 6.x、Rsbuild
- 部署：Docker、Docker Compose

## 快速部署

部署前先确认宿主机的 `25` 和 `8080` 端口没有被占用。

```bash
docker compose up -d --build
```

启动后访问：

- 后台地址：`http://服务器IP:8080`
- 接口文档：`http://服务器IP:8080/docs`
- 开放接口 Markdown：`http://服务器IP:8080/docs/api.md`

默认管理员账号来自 `/Users/wanz/web/my/gpt/mx-mail-api/docker/config.yaml`：

```text
用户名：admin
密码：admin123456
```

首次部署后建议立即修改：

- `/Users/wanz/web/my/gpt/mx-mail-api/docker/config.yaml` 中的 `auth.jwt_secret`
- 默认管理员密码
- `smtp.hostname`
- Postgres 用户名和密码

## 域名解析

假设要接收 `example.com` 的邮件，并且服务部署在 `mail.example.com`：

```dns
@     MX    mail.example.com
mail  A     服务器IP
```

如果使用 Docker Compose 默认配置，SMTP 对外端口是 `25`，容器内服务端口是 `2525`。

新增域名时，后台会生成一条 TXT 验证记录。按页面给出的记录名和值添加解析，解析生效后再提交保存。

## 配置文件

Docker Compose 默认挂载：

```text
/Users/wanz/web/my/gpt/mx-mail-api/docker/config.yaml -> /app/config.yaml
```

核心配置示例：

```yaml
http:
  addr: ":8080"

smtp:
  addr: ":2525"
  hostname: "mail.example.com"

database:
  dsn: "host=postgres user=mx_mail_api password=mx_mail_api_password dbname=mx_mail_api port=5432 sslmode=disable"

auth:
  jwt_secret: "change-me-before-deploy"
  token_ttl_hours: 24

admin:
  initial_user:
    username: "admin"
    password: "admin123456"
```

`smtp.hostname` 应设置为实际 MX 指向的主机名，例如 `mail.example.com`。

## 开放接口

开放接口使用用户自己的 API Key 认证。登录后台后，可在右上角用户菜单中生成或重置 API Key。

常用接口：

- `POST /openapi/temporary-mailboxes`：申请邮箱
- `GET /openapi/temporary-mailboxes/latest-message?address=xxx@example.com`：获取指定邮箱最新邮件

完整说明见后台页面 `/docs` 或仓库根目录 `/Users/wanz/web/my/gpt/mx-mail-api/API.md`。

## 本地开发

后端：

```bash
cd server
go test ./...
go run .
```

前端：

```bash
cd web
pnpm install
pnpm run dev
```

前端开发服务默认运行在 `http://localhost:3000`。

## 常见问题

### 域名验证失败

先确认 TXT 是否已生效：

```bash
dig +short TXT 你的验证记录名
```

如果本地能查到但服务仍验证失败，通常是容器运行环境的 DNS 未及时同步。当前服务会优先尝试系统 DNS，失败后再使用 `223.5.5.5`、`1.1.1.1`、`8.8.8.8` 查询。

### Docker 启动时 25 端口被占用

释放宿主机 25 端口，或临时调整 `/Users/wanz/web/my/gpt/mx-mail-api/docker-compose.yaml`：

```yaml
ports:
  - "8080:8080"
  - "2525:2525"
```

生产环境接收公网邮件时仍建议使用标准 SMTP 端口 `25`。
