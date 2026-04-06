# 开发与构建说明

本文档说明 `gmbox` 在本地开发、前后端构建、测试验证和发布前检查时的推荐流程。

## 环境要求

- `Go 1.26.1`
- `Node.js` 与 `npm`
- 可选数据库：`sqlite`、`postgres`、`mysql`

默认开发配置可从仓库根目录的 `config.example.yaml` 复制生成：

```bash
cp config.example.yaml config.yaml
```

## 本地开发

### 安装依赖

```bash
make deps
```

该命令等价于：

```bash
npm install
```

### 启动服务端

```bash
make run
```

该命令等价于：

```bash
go run ./cmd/server
```

服务启动后默认监听 `:8080`。

### 前端单独开发

如果只需要调试前端资源，可以运行：

```bash
npm run dev
```

前端使用 `web/vite.config.ts` 作为 Vite 配置文件。

## 构建流程

### 前端构建

```bash
make web-build
```

该命令等价于：

```bash
npm run build
```

输出目录为 `frontend/dist`。

### 服务端构建

```bash
make server-build
```

该命令等价于：

```bash
go build -o ./gmbox ./cmd/server
```

注意：服务端会通过 `Go embed` 引用 `frontend/dist`，因此独立执行服务端构建前，应确保前端产物已存在且是最新版本。

### 完整构建

```bash
make build
```

该命令会按以下顺序执行：

1. `make web-build`
2. `make server-build`

这个顺序不能颠倒，否则服务端最终二进制可能缺少最新前端资源。

## 测试与验证

### 后端测试

运行全部 Go 测试：

```bash
make test
```

等价命令：

```bash
go test ./...
```

如果只验证单个包，可以直接运行：

```bash
go test ./internal/httpapi
```

如果只运行某个测试：

```bash
go test ./internal/httpapi -run TestName -v
```

### 前端验证

当前仓库未配置独立的前端单元测试命令，前端改动后的主要验证方式是：

```bash
npm run build
```

### 跨栈改动验证

如果同时修改了前后端，或修改了会影响嵌入资源的流程，建议至少执行：

```bash
make build
go test ./...
```

## 常用清理命令

```bash
make clean
```

该命令会清理：

- `./gmbox`
- `./frontend/dist`

不会删除用户数据文件或配置文件。

## 发布前检查建议

发布前建议按下面顺序检查：

1. 确认 `config.yaml` 中数据库、监听地址和密钥配置正确
2. 执行 `make test`
3. 执行 `make build`
4. 启动 `./gmbox`，确认首页、登录和核心邮件流程正常
5. 如果使用微软 OAuth，确认 Azure 应用中的 Redirect URI 与实际访问地址一致
