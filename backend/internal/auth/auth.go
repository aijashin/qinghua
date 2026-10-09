package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"time"
	"zhiwei/backend/internal/store"
)

type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Disabled    bool   `json:"disabled"`
}
type Service struct{ DB *sql.DB }

func Token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func Hash(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func (s Service) Create(ctx context.Context, username, password, name, role string) (User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 80 || strings.ContainsAny(username, " \t\r\n") {
		return User{}, errors.New("用户名应为3至80个非空白字符")
	}
	if len(password) < 10 || len(password) > 72 {
		return User{}, errors.New("密码需要10至72字节")
	}
	if role != "admin" && role != "teacher" && role != "student" {
		return User{}, errors.New("无效角色")
	}
	if name == "" {
		name = username
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), 12)
	if e != nil {
		return User{}, e
	}
	u := User{ID: store.ID(), Username: username, DisplayName: name, Role: role}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO users VALUES(?,?,?,?,?,0,?)`, u.ID, u.Username, string(hash), u.DisplayName, u.Role, store.Now())
	return u, e
}
func (s Service) Login(ctx context.Context, username, password string) (User, string, string, error) {
	var u User
	var hash string
	e := s.DB.QueryRowContext(ctx, `SELECT id,username,display_name,role,disabled,password_hash FROM users WHERE username=?`, username).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Disabled, &hash)
	if e != nil || u.Disabled || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, "", "", errors.New("用户名或密码不正确")
	}
	token, csrf := Token(), Token()
	_, e = s.DB.ExecContext(ctx, `INSERT INTO sessions VALUES(?,?,?,?)`, Hash(token), u.ID, csrf, time.Now().UTC().Add(7*24*time.Hour).Format(time.RFC3339Nano))
	return u, token, csrf, e
}
func (s Service) Session(ctx context.Context, token string) (User, string, error) {
	var u User
	var csrf string
	e := s.DB.QueryRowContext(ctx, `SELECT u.id,u.username,u.display_name,u.role,u.disabled,s.csrf FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>? AND u.disabled=0`, Hash(token), store.Now()).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Disabled, &csrf)
	return u, csrf, e
}
