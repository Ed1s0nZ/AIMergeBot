package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed dist
var assets embed.FS

func Register(r *gin.Engine) {
	root, _ := fs.Sub(assets, "dist")
	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet || strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/webhook" {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		p := strings.TrimPrefix(path.Clean(c.Request.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(root, p); err != nil {
			if strings.Contains(path.Base(p), ".") {
				c.Status(404)
				return
			}
			p = "index.html"
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			c.Status(404)
			return
		}
		contentType := "application/octet-stream"
		switch path.Ext(p) {
		case ".html":
			contentType = "text/html; charset=utf-8"
			c.Header("Cache-Control", "no-cache")
		case ".js":
			contentType = "text/javascript; charset=utf-8"
			c.Header("Cache-Control", "public,max-age=31536000,immutable")
		case ".css":
			contentType = "text/css; charset=utf-8"
			c.Header("Cache-Control", "public,max-age=31536000,immutable")
		}
		c.Data(200, contentType, data)
	})
}
