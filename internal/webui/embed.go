// Package webui：把仪表盘静态页编译进二进制（go:embed）。
//
// 是什么：embed.FS 在编译时打包 internal/webui/index.html。
// 干什么：init 里读出字节切片到 IndexHTML，handler 直接 c.Data 返回。
// 怎么用：修改 index.html 后重新 go build；无需单独部署 nginx 静态目录。
package webui

import "embed"

//go:embed index.html
var files embed.FS // 编译期嵌入的小文件系统

// IndexHTML 启动时由 init 填充，供 ServeDashboard 返回给浏览器。
var IndexHTML []byte

func init() {
	b, err := files.ReadFile("index.html") // 路径相对于 embed 注释里的文件
	if err != nil {
		panic("webui: " + err.Error()) // 嵌入失败说明构建有问题，直接 panic
	}
	IndexHTML = b
}
