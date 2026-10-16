package repository

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID           string
	Username     string
	PasswordHash string
}
type UserRepository struct{ DB *pgxpool.Pool }

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	u := &User{}
	err := r.DB.QueryRow(ctx, `SELECT id::text,username,password_hash FROM users WHERE username=$1`, username).Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}
func (r *UserRepository) CreateOrUpdate(ctx context.Context, username, passwordHash string) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO users(username,password_hash) VALUES($1,$2) ON CONFLICT(username) DO UPDATE SET password_hash=EXCLUDED.password_hash`, username, passwordHash)
	return err
}
