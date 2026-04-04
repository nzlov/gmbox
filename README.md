# gmbox

单管理员邮件客户端，使用 `Go + Gin + Vue3`，默认支持 `sqlite`，并兼容 `postgres`、`mysql`。

## 当前已实现

- `config.yaml` + 默认值回落 + 环境变量强覆盖
- 首次启动导入默认管理员到数据库
- `JWT + HttpOnly Cookie` 登录
- 邮箱账户 CRUD
- 常见邮箱服务商自动配置与自定义服务商
- 微软 OAuth 邮箱接入
- 非 OAuth 邮箱批量导入
- 邮箱密码 `AES-GCM` 加密存储
- `cron/v3` 定时同步
- `IMAP` 多文件夹增量收件
- `POP3 UIDL` 去重收件
- `SMTP` 发信接口
- 附件解析、落盘与下载
- 邮件详情页、正文展示、已读未读/删除/移动操作
- Gmail 风格前端基础页面
- Gmail 风格写信页
- Go `embed` 嵌入前端构建产物
- Docker 多阶段构建

## 本地启动

```bash
make deps
make build
make run
```

如果只想执行单独步骤：

- `make web-build`：构建前端静态资源
- `make server-build`：构建服务端二进制 `./gmbox`
- `make test`：运行 Go 测试
- `make clean`：清理本地产物

默认地址：`http://127.0.0.1:8080`

默认管理员：读取 `config.yaml` 中的 `auth.init_username` 和 `auth.init_password`，仅首次启动导入。

## 微软 OAuth 配置

如需启用 Outlook / Microsoft 365 OAuth，请在 `config.yaml` 或环境变量中提供：

- `microsoft_oauth.tenant_id` / `MICROSOFT_OAUTH_TENANT_ID`
- `microsoft_oauth.client_id` / `MICROSOFT_OAUTH_CLIENT_ID`
- `microsoft_oauth.client_secret` / `MICROSOFT_OAUTH_CLIENT_SECRET`
- `microsoft_oauth.redirect_url` / `MICROSOFT_OAUTH_REDIRECT_URL`

`redirect_url` 现在支持两种模式：

- 显式配置：如果配置了 `microsoft_oauth.redirect_url`，系统始终使用该值
- 自动推导：如果未配置 `microsoft_oauth.redirect_url`，系统会按当前访问地址自动生成 `当前站点地址/oauth/microsoft/callback`

兼容说明：

- 如果显式配置成 `/oauth/microsoft/callback`，前端会走 PKCE 流程
- 如果显式配置成 `/api/accounts/oauth/microsoft/callback`，系统会自动切回旧服务端回调兼容流，避免已有配置升级后立即失效

示例：

- 当前通过 `http://127.0.0.1:8080` 访问时，自动回调地址为 `http://127.0.0.1:8080/oauth/microsoft/callback`
- 当前通过 `https://mail.example.com` 访问时，自动回调地址为 `https://mail.example.com/oauth/microsoft/callback`

推荐做法：

- 单域名直连部署时，可以不配 `redirect_url`，直接使用自动推导
- 反向代理或公网域名部署时，确保代理正确透传 `X-Forwarded-Proto` 和 `X-Forwarded-Host`
- 如果存在多个访问域名，或微软应用只登记了固定回调地址，建议显式配置 `redirect_url`

Azure 应用注册里需要把实际使用的回调地址加入 Redirect URI 白名单。若你依赖自动推导，请把用户真实访问的域名对应的 `/oauth/microsoft/callback` 登记进去。
