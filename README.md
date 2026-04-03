# gmbox

单管理员邮件客户端，使用 `Go + Gin + Vue3`，默认支持 `sqlite`，并兼容 `postgres`、`mysql`。

## 当前已实现

- `config.yaml` + 默认值回落 + 环境变量强覆盖
- 首次启动导入默认管理员到数据库
- `JWT + HttpOnly Cookie` 登录
- 邮箱账户 CRUD
- 邮箱密码 `AES-GCM` 加密存储
- `cron/v3` 定时同步骨架
- Gmail 风格前端基础页面
- Go `embed` 嵌入前端构建产物
- Docker 多阶段构建

## 本地启动

```bash
npm install
npm run build
go run ./cmd/server
```

默认地址：`http://127.0.0.1:8080`

默认管理员：读取 `config.yaml` 中的 `auth.init_username` 和 `auth.init_password`，仅首次启动导入。
