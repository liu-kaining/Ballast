// Package store 实现 PostgreSQL 数据访问层。
// v0.1 不引 ORM，直接基于 github.com/jackc/pgx/v5。
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store 聚合所有表的访问器。
type Store struct {
	pool     *pgxpool.Pool
	Sessions *SessionStore
	Audit    *AuditStore
	Skills   *SkillStore
}

// New 用 DSN 构造连接池并返回 Store。
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("build pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	s := &Store{pool: pool}
	s.Sessions = &SessionStore{pool: pool}
	s.Audit = &AuditStore{pool: pool}
	s.Skills = &SkillStore{pool: pool}
	return s, nil
}

// Close 释放连接池。
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// Pool 暴露底层连接池，供需要原生查询的调用方使用。
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Ping verifies that PostgreSQL is reachable.
func (s *Store) Ping(ctx context.Context) error {
	if s.pool == nil {
		return fmt.Errorf("store is closed")
	}
	return s.pool.Ping(ctx)
}
