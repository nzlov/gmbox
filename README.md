# gmbox

`gmbox` 是一个单管理员邮件客户端，使用 `Go + Gin + Vue 3 + Quasar` 构建，默认支持 `sqlite`，同时兼容 `postgres` 和 `mysql`。

项目提供邮件账户管理、收信同步、邮件详情、附件下载、发信与微软 OAuth 接入能力，并将前端构建产物通过 `Go embed` 打包进服务端二进制，便于单文件部署。

## Features

- 单管理员登录与会话管理，使用 `JWT + HttpOnly Cookie`
- 邮箱账户 CRUD，支持常见服务商自动配置和自定义配置
- 微软 OAuth 邮箱接入
- `IMAP` 增量收件、`POP3 UIDL` 去重收件
- `SMTP` 发信
- 邮件详情展示、已读未读、删除、移动等基础操作
- 附件解析、落盘与下载
- 定时同步任务
- 前端静态资源内嵌到 Go 服务端，适合简化部署
- Docker 多阶段构建

## Tech Stack

- Backend: `Go`, `Gin`, `GORM`
- Frontend: `Vue 3`, `TypeScript`, `Vite`, `Quasar`
- Database: `sqlite` / `postgres` / `mysql`

## Quick Start

### 1. 安装依赖

```bash
make deps
```

### 2. 准备配置

复制示例配置并按需修改：

```bash
cp config.example.yaml config.yaml
```

默认监听地址是 `http://127.0.0.1:8080`。

### 3. 构建并运行

```bash
make build
make run
```

首次启动时，程序会根据 `auth.init_username` 初始化管理员账号，并将随机密码输出到启动日志中。

## Build Commands

- `make deps`：安装前端依赖
- `make web-build`：构建前端静态资源
- `make server-build`：构建服务端二进制 `./gmbox`
- `make build`：先构建前端，再构建服务端
- `make run`：本地启动服务
- `make test`：运行全部 Go 测试
- `make clean`：清理本地产物

更多本地开发与构建细节见 `docs/development-build.md`。

## Configuration

核心配置位于 `config.yaml`，可参考 `config.example.yaml`。

常用配置项：

- `app.addr`：服务监听地址
- `app.secret_key`：会话和签名相关密钥
- `auth.init_username`：首次启动时初始化的管理员用户名
- `db.driver` / `db.dsn`：数据库类型与连接配置
- `mail.sync_cron`：定时同步表达式
- `log.level`：日志等级

### 日志等级

项目使用 Go 官方 `slog`。

- `log.level=info`：默认等级
- `log.level=debug`：输出更详细的调试日志，包括 IMAP、SMTP、微软 OAuth 等交互日志
- `log.level=warn|error`：只输出更高等级日志

也可以通过环境变量 `LOG_LEVEL` 覆盖。

### 微软 OAuth

如需启用 Outlook / Microsoft 365 OAuth，请在 `config.yaml` 或环境变量中提供：

- `microsoft_oauth.tenant_id` / `MICROSOFT_OAUTH_TENANT_ID`
- `microsoft_oauth.client_id` / `MICROSOFT_OAUTH_CLIENT_ID`
- `microsoft_oauth.client_secret` / `MICROSOFT_OAUTH_CLIENT_SECRET`
- `microsoft_oauth.redirect_url` / `MICROSOFT_OAUTH_REDIRECT_URL`

`redirect_url` 支持两种模式：

- 显式配置固定回调地址
- 留空时按当前访问地址自动推导为 `当前站点地址/oauth/microsoft/callback`

如果通过反向代理部署，请确保代理正确透传 `X-Forwarded-Proto` 和 `X-Forwarded-Host`。

## Project Structure

```text
.
|-- cmd/server           服务端入口
|-- internal/            后端业务代码
|-- web/src              前端源码
|-- frontend/dist        前端构建产物
|-- docs/                补充文档
|-- config.example.yaml  示例配置
`-- Makefile             常用构建命令
```

## Development Docs

- `docs/development-build.md`：本地开发、构建顺序、测试与发布前检查
- `docs/v1.0.0.md`：版本说明
- `docs/v1.1.0.md`：版本说明

## License

本项目基于 `MIT` License 发布，详见仓库根目录的 `LICENSE` 文件。
