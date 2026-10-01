// Package db provides Postgres connection and order storage helpers for shop services
package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Order struct {
	ID          int64     `json:"id"`
	Description string    `json:"description"`
	Processed   bool      `json:"processed"`
	CreatedAt   time.Time `json:"created_at"`
}

const schema = `
CREATE TABLE IF NOT EXISTS orders (
	id          BIGSERIAL PRIMARY KEY,
	description TEXT NOT NULL,
	processed   BOOLEAN NOT NULL DEFAULT FALSE,
	created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS orders_unprocessed_idx
	ON orders (id) WHERE NOT processed;
`

func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := buildDSN()

	var lastErr error
	for attempt := 1; attempt <= 30; attempt++ {
		pool, err := pgxpool.New(ctx, dsn)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				return pool, nil
			}
			pool.Close()
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("не удалось подключиться к postgres после 30 попыток: %w", lastErr)
}

func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, schema)
	return err
}

func CreateOrder(ctx context.Context, pool *pgxpool.Pool, description string) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO orders (description) VALUES ($1) RETURNING id`,
		description,
	).Scan(&id)
	return id, err
}

func ListOrders(ctx context.Context, pool *pgxpool.Pool) ([]Order, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, description, processed, created_at FROM orders ORDER BY id DESC LIMIT 100`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.Description, &o.Processed, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func ClaimNextOrder(ctx context.Context, pool *pgxpool.Pool) (*Order, error) {
	var o Order
	err := pool.QueryRow(ctx, `
		UPDATE orders SET processed = TRUE
		WHERE id = (
			SELECT id FROM orders
			WHERE NOT processed
			ORDER BY id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, description, processed, created_at
	`).Scan(&o.ID, &o.Description, &o.Processed, &o.CreatedAt)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &o, nil
}

func buildDSN() string {
	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "app")
	pass := os.Getenv("DB_PASSWORD")
	name := getenv("DB_NAME", "app")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=prefer", user, pass, host, port, name)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}