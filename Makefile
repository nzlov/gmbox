.PHONY: help deps web-build server-build build run test clean

GO ?= go
NPM ?= npm
BINARY ?= gmbox

help:
	@printf '%s\n' \
		'可用目标:' \
		'  make deps         安装前端依赖' \
		'  make web-build    构建前端静态资源' \
		'  make server-build 构建服务端二进制' \
		'  make build        先构建前端，再构建服务端' \
		'  make run          本地启动服务' \
		'  make test         运行 Go 测试' \
		'  make clean        清理构建产物'

# 先安装前端依赖，避免构建时缺少本地工具链。
deps:
	$(NPM) install

# 前端资源会被 Go embed 打进服务端，因此需要先完成前端构建。
web-build:
	$(NPM) run build

# 服务端依赖 frontend/dist，独立构建时默认要求前端产物已存在。
server-build:
	$(GO) build -o ./$(BINARY) ./cmd/server

# 保持与当前 Docker 和 README 一致的构建顺序，避免 embed 缺少静态资源。
build: web-build server-build

run:
	$(GO) run ./cmd/server

test:
	$(GO) test ./...

# 只清理本地可再生产物，不碰用户数据和配置文件。
clean:
	rm -f ./$(BINARY)
	rm -rf ./frontend/dist
