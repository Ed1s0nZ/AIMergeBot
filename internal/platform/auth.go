package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

var ErrCredentials = errors.New("invalid credentials")
var ErrLastAdmin = errors.New("cannot disable or demote the last administrator")

func validateAccount(username, password, role string) error {
	if len(username) < 3 || len(username) > 64 {
		return fmt.Errorf("username must have 3–64 characters")
	}
	for _, r := range username {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' {
			return fmt.Errorf("invalid username")
		}
	}
	if len(password) < 12 || len(password) > 72 {
		return fmt.Errorf("password must have 12–72 bytes")
	}
	if role != "admin" && role != "member" {
		return fmt.Errorf("invalid role")
	}
	return nil
}

func (s *Store) CreateUser(ctx context.Context, username, password, role string) (User, error) {
	var u User
	username = strings.TrimSpace(username)
	if err := validateAccount(username, password, role); err != nil {
		return u, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return u, err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO platform_users(username,password_hash,role,created_at) VALUES(?,?,?,?)`, username, string(hash), role, now())
	if err != nil {
		return u, err
	}
	u.ID, err = res.LastInsertId()
	u.Username = username
	u.Role = role
	return u, err
}

func (s *Store) Bootstrap(ctx context.Context, username, password string) error {
	var count int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if username == "" || password == "" {
		return fmt.Errorf("empty database requires AIM_ADMIN_USERNAME and AIM_ADMIN_PASSWORD (at least 12 bytes)")
	}
	_, err := s.CreateUser(ctx, username, password, "admin")
	return err
}

func tokenHash(raw string) string { h := sha256.Sum256([]byte(raw)); return hex.EncodeToString(h[:]) }

func (s *Store) Login(ctx context.Context, username, password string) (User, string, error) {
	var u User
	var hash string
	err := s.DB.QueryRowContext(ctx, `SELECT id,username,role,disabled,password_hash FROM platform_users WHERE username=?`, username).Scan(&u.ID, &u.Username, &u.Role, &u.Disabled, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return u, "", err
	}
	// A fixed valid hash keeps unknown-account checks comparable in cost.
	if hash == "" {
		hash = "$2a$10$7EqJtq98hPqEX7fNZaFWoO5uVMSHQWYvRH7c6MEV8jCpLPms.o8xe"
	}
	check := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil || check != nil || u.Disabled {
		return User{}, "", ErrCredentials
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return u, "", err
	}
	token := hex.EncodeToString(raw)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO platform_sessions(token_hash,user_id,expires_at) VALUES(?,?,?)`, tokenHash(token), u.ID, time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339Nano))
	return u, token, err
}

func (s *Store) Session(ctx context.Context, token string) (User, error) {
	var u User
	if len(token) != 64 {
		return u, ErrCredentials
	}
	var expires string
	err := s.DB.QueryRowContext(ctx, `SELECT u.id,u.username,u.role,u.disabled,s.expires_at FROM platform_sessions s JOIN platform_users u ON u.id=s.user_id WHERE s.token_hash=?`, tokenHash(token)).Scan(&u.ID, &u.Username, &u.Role, &u.Disabled, &expires)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return u, ErrCredentials
		}
		return u, err
	}
	t, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return u, err
	}
	if u.Disabled || !t.After(time.Now()) {
		return u, ErrCredentials
	}
	return u, nil
}

func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM platform_sessions WHERE token_hash=?`, tokenHash(token))
	return err
}

func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,username,role,disabled FROM platform_users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []User{}
	for rows.Next() {
		var u User
		if err = rows.Scan(&u.ID, &u.Username, &u.Role, &u.Disabled); err != nil {
			return nil, err
		}
		items = append(items, u)
	}
	return items, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, id int64, role string, disabled bool, password string) error {
	if role != "admin" && role != "member" {
		return fmt.Errorf("invalid role")
	}
	var hash []byte
	var err error
	if password != "" {
		if len(password) < 12 || len(password) > 72 {
			return fmt.Errorf("password must have 12–72 bytes")
		}
		hash, err = bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldRole string
	var oldDisabled bool
	if err = tx.QueryRowContext(ctx, `SELECT role,disabled FROM platform_users WHERE id=?`, id).Scan(&oldRole, &oldDisabled); err != nil {
		return err
	}
	if oldRole == "admin" && !oldDisabled && (role != "admin" || disabled) {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_users WHERE role='admin' AND disabled=0`).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastAdmin
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE platform_users SET role=?,disabled=? WHERE id=?`, role, disabled, id); err != nil {
		return err
	}
	if password != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE platform_users SET password_hash=? WHERE id=?`, string(hash), id); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM platform_sessions WHERE user_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
