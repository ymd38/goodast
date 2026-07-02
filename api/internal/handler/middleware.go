package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit はリクエストボディの読み取りを maxBytes までに制限するミドルウェア。
// 巨大ボディによるメモリ/CPU の過剰消費を防ぐ。上限超過はハンドラ側の
// ShouldBindJSON がエラーを返し 400 になる（読み取り自体が上限で打ち切られる）。
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
