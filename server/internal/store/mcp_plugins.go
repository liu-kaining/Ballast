package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ballast/ballast-server/internal/domain"
)

type MCPPluginStore struct {
	pool *pgxpool.Pool
}

// Upsert 插入或更新一个 MCP 插件资产。
func (s *MCPPluginStore) Upsert(ctx context.Context, plugin *domain.MCPPlugin) error {
	env, err := json.Marshal(plugin.Env)
	if err != nil {
		return fmt.Errorf("marshal mcp env: %w", err)
	}
	const q = `
		INSERT INTO ballast_mcp_plugins
			(plugin_id, name, command, args, env, is_active, updated_by)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
		ON CONFLICT (plugin_id) DO UPDATE SET
			name = EXCLUDED.name,
			command = EXCLUDED.command,
			args = EXCLUDED.args,
			env = EXCLUDED.env,
			is_active = EXCLUDED.is_active,
			updated_by = EXCLUDED.updated_by,
			updated_at = CURRENT_TIMESTAMP`
	_, err = s.pool.Exec(ctx, q,
		plugin.PluginID, plugin.Name, plugin.Command, plugin.Args,
		string(env), plugin.IsActive, plugin.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("upsert mcp plugin: %w", err)
	}
	return nil
}

// Get 按 ID 查询一个 MCP 插件资产。
func (s *MCPPluginStore) Get(ctx context.Context, id string) (*domain.MCPPlugin, error) {
	const q = `
		SELECT plugin_id, name, command, args, env, is_active, updated_by, updated_at
		FROM ballast_mcp_plugins WHERE plugin_id = $1`
	var plugin domain.MCPPlugin
	var env []byte
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&plugin.PluginID, &plugin.Name, &plugin.Command, &plugin.Args,
		&env, &plugin.IsActive, &plugin.UpdatedBy, &plugin.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get mcp plugin: %w", err)
	}
	if len(env) != 0 {
		if err := json.Unmarshal(env, &plugin.Env); err != nil {
			return nil, fmt.Errorf("decode mcp env: %w", err)
		}
	}
	if plugin.Env == nil {
		plugin.Env = map[string]string{}
	}
	return &plugin, nil
}

// List 列出所有 MCP 插件资产。
func (s *MCPPluginStore) List(ctx context.Context) ([]*domain.MCPPlugin, error) {
	const q = `
		SELECT plugin_id, name, command, args, env, is_active, updated_by, updated_at
		FROM ballast_mcp_plugins ORDER BY name ASC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list mcp plugins: %w", err)
	}
	defer rows.Close()
	var out []*domain.MCPPlugin
	for rows.Next() {
		var plugin domain.MCPPlugin
		var env []byte
		if err := rows.Scan(
			&plugin.PluginID, &plugin.Name, &plugin.Command, &plugin.Args,
			&env, &plugin.IsActive, &plugin.UpdatedBy, &plugin.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan mcp plugin: %w", err)
		}
		if len(env) != 0 {
			if err := json.Unmarshal(env, &plugin.Env); err != nil {
				return nil, fmt.Errorf("decode mcp env: %w", err)
			}
		}
		if plugin.Env == nil {
			plugin.Env = map[string]string{}
		}
		out = append(out, &plugin)
	}
	return out, rows.Err()
}
