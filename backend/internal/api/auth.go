package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"procurewatch/internal/store"
)

type session struct {
	UserID      int64
	Username    string
	DisplayName string
	Role        string
	ExpiresAt   time.Time
}

type Auth struct {
	mu       sync.RWMutex
	sessions map[string]session
	st       *store.Store
}

func NewAuth(st *store.Store) *Auth {
	return &Auth{sessions: map[string]session{}, st: st}
}

func (a *Auth) issue(userID int64, username, display, role string) string {
	b := make([]byte, 24)
	rand.Read(b)
	token := hex.EncodeToString(b)
	a.mu.Lock()
	a.sessions[token] = session{
		UserID: userID, Username: username, DisplayName: display, Role: role,
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}
	a.mu.Unlock()
	return token
}

func (a *Auth) resolve(token string) (session, bool) {
	a.mu.RLock()
	s, ok := a.sessions[token]
	a.mu.RUnlock()
	if !ok || time.Now().After(s.ExpiresAt) {
		return session{}, false
	}
	return s, true
}

func (a *Auth) Login(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}
	id, hash, display, role, err := a.st.GetUserByUsername(c.Request.Context(), in.Username)
	if err == store.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !checkPassword(hash, in.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	token := a.issue(id, in.Username, display, role)
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  gin.H{"id": id, "username": in.Username, "display_name": display, "role": role},
	})
}

func (a *Auth) Me(c *gin.Context) {
	s := MustGetSession(c)
	c.JSON(http.StatusOK, gin.H{"id": s.UserID, "username": s.Username,
		"display_name": s.DisplayName, "role": s.Role})
}

// Middleware 校验 Bearer 令牌并注入会话。
func (a *Auth) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		s, ok := a.resolve(header[len(prefix):])
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期，请重新登录"})
			return
		}
		c.Set("session", s)
		c.Next()
	}
}

func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := MustGetSession(c)
		if s.Role != role && s.Role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "需要 " + role + " 权限"})
			return
		}
		c.Next()
	}
}

func checkPassword(hash, password string) bool {
	return bcryptCompare(hash, password)
}
