package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Session struct {
	Token     string
	CSRFToken string
	UserID    *string
	ExpiresAt time.Time
}
type SessionStore struct {
	DB  *pgxpool.Pool
	TTL time.Duration
}

var ErrSessionNotFound = errors.New("session not found")

func NewOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *SessionStore) Create(ctx context.Context, userID *string) (*Session, error) {
	token, err := NewOpaqueToken()
	if err != nil {
		return nil, err
	}
	csrf, err := NewOpaqueToken()
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(s.TTL)
	_, err = s.DB.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,$3,$4)`, tokenHash(token), userID, csrf, expires)
	if err != nil {
		return nil, err
	}
	return &Session{Token: token, CSRFToken: csrf, UserID: userID, ExpiresAt: expires}, nil
}
func (s *SessionStore) Get(ctx context.Context, token string) (*Session, error) {
	var userID *string
	var csrf string
	var expires time.Time
	err := s.DB.QueryRow(ctx, `SELECT user_id::text, csrf_token, expires_at FROM sessions WHERE token_hash=$1 AND expires_at > now()`, tokenHash(token)).Scan(&userID, &csrf, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Session{Token: token, CSRFToken: csrf, UserID: userID, ExpiresAt: expires}, nil
}
func (s *SessionStore) Delete(ctx context.Context, token string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash(token))
	return err
}
func (s *SessionStore) CleanupExpired(ctx context.Context) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	return err
}
