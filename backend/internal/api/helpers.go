package api

import (
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func bcryptCompare(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// MustGetSession 从 gin.Context 取会话（中间件已保证存在）。
func MustGetSession(c *gin.Context) session {
	v, ok := c.Get("session")
	if !ok {
		return session{}
	}
	return v.(session)
}
