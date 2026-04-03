package frontend

import (
	"embed"
	"io/fs"
)

// assets 保存前端构建后的静态资源。
//
//go:embed dist/*
var assets embed.FS

// Sub 返回 dist 子目录，便于 HTTP 层直接挂载静态资源。
func Sub() fs.FS {
	dist, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return dist
}
