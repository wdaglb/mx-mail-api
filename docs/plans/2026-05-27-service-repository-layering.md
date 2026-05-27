# 后端 service/repository 分层方案

## 已确认范围

- 本次只改后端，不调整前端页面和前端 API 调用封装。
- API 和 SMTP 收件链路都纳入分层改造。
- 目标结构为 `internal/service` + `internal/repository`。
- API 层和 SMTP 协议层不再直接操作 GORM。
- 保持现有 HTTP 接口、SMTP 命令行为、数据库模型和响应字段兼容。

## 分层目标

- `internal/api` 只负责 HTTP 参数绑定、状态码、错误响应和 DTO 输出。
- `internal/smtpserver` 只负责 SMTP 命令状态机、协议响应和 DATA 读取。
- `internal/service` 承载用户、认证、域名、收件记录和 SMTP 收件策略等业务流程。
- `internal/repository` 封装 GORM 查询和写入，避免上层直接依赖数据库细节。
- `internal/storage` 保留数据模型、基础存储打开逻辑和纯规则函数。

## 计划拆分

### repository

- 新增用户仓储，负责用户查询、创建、更新、删除和 API Key 哈希扫描。
- 新增域名仓储，负责域名列表、查询、创建、更新、删除，以及 SMTP 使用的接受域名读取。
- 新增邮件仓储，负责收件记录写入、列表和单条查询。

### service

- 新增用户服务，负责首个管理员初始化、登录、用户管理和 API Key 重置。
- 新增域名服务，负责普通用户与管理员的域名可见性和管理权限。
- 新增邮件服务，负责收件记录可见性、正文解析和 SMTP 入库。
- 新增 SMTP 策略服务，负责 RCPT TO 域名验收和 DATA 入库动作。

## 验证计划

```bash
cd /Users/wanz/web/my/gpt/mx-mail-api/server
go test ./...
```

## 风险控制

- 不修改数据库表结构，避免引入迁移风险。
- 不修改 API 响应结构，避免前端联动风险。
- 不修改 SMTP 命令响应文本，避免破坏现有协议测试。
- 拆分后优先复用现有测试，必要时只补充 service 层关键测试。
