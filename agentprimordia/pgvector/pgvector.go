package pgvector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Config 是 pgvector 客户端配置
type Config struct {
	Host       string `json:"host"`       // 默认 "localhost"
	Port       int    `json:"port"`       // 默认 5432
	Database   string `json:"database"`   // 数据库名称
	User       string `json:"user"`       // 用户名
	Password   string `json:"-"`          // 密码（不序列化）
	TableName  string `json:"tableName"`  // 向量表名（默认 "ap_vectors"）
	VectorSize int    `json:"vectorSize"` // 向量维度（默认 1536）
	SSLMode    string `json:"sslMode"`    // SSL 模式（默认 "disable"）
}

// SearchResult pgvector 搜索结果
type SearchResult struct {
	ID       string         `json:"id"`
	Score    float32        `json:"score"`
	Text     string         `json:"text"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Client 是 pgvector 向量数据库客户端
// 使用 PostgreSQL + pgvector 扩展进行向量存储和相似度搜索
type Client struct {
	config Config
	db     *sql.DB
}

// NewClient 创建新的 pgvector 客户端
func NewClient(config Config) (*Client, error) {
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.Port == 0 {
		config.Port = 5432
	}
	if config.TableName == "" {
		config.TableName = "ap_vectors"
	}
	if config.VectorSize == 0 {
		config.VectorSize = 1536
	}
	if config.SSLMode == "" {
		config.SSLMode = "disable"
	}

	connStr := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		config.Host, config.Port, config.Database, config.User, config.Password, config.SSLMode,
	)

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("连接 PostgreSQL 失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("PostgreSQL 连接测试失败: %w", err)
	}

	client := &Client{config: config, db: db}

	if err := client.ensureSchema(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化表结构失败: %w", err)
	}

	return client, nil
}

func (c *Client) ensureSchema(ctx context.Context) error {
	if _, err := c.db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		return fmt.Errorf("创建 pgvector 扩展失败（请确认已安装 pgvector）: %w", err)
	}

	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			text TEXT NOT NULL DEFAULT '',
			metadata JSONB DEFAULT '{}',
			vector vector(%d) NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`, c.config.TableName, c.config.VectorSize)

	if _, err := c.db.ExecContext(ctx, createSQL); err != nil {
		return fmt.Errorf("创建向量表失败: %w", err)
	}

	indexName := fmt.Sprintf("%s_vector_idx", c.config.TableName)
	indexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS %s ON %s
		USING ivfflat (vector vector_cosine_ops)
		WITH (lists = 100)`, indexName, c.config.TableName)

	c.db.ExecContext(ctx, indexSQL)

	return nil
}

// Insert 插入一条向量记录
func (c *Client) Insert(ctx context.Context, id string, vector []float32, text string, metadata map[string]any) error {
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("序列化元数据失败: %w", err)
	}

	vectorStr := floatSliceToVector(vector)
	query := fmt.Sprintf(`
		INSERT INTO %s (id, text, metadata, vector)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			text = EXCLUDED.text,
			metadata = EXCLUDED.metadata,
			vector = EXCLUDED.vector`,
		c.config.TableName)

	_, err = c.db.ExecContext(ctx, query, id, text, string(metaJSON), vectorStr)
	return err
}

// BatchInsert 批量插入向量记录
func (c *Client) BatchInsert(ctx context.Context, ids []string, vectors [][]float32, texts []string, metadatas []map[string]any) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := fmt.Sprintf(`
		INSERT INTO %s (id, text, metadata, vector)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			text = EXCLUDED.text,
			metadata = EXCLUDED.metadata,
			vector = EXCLUDED.vector`,
		c.config.TableName)

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range ids {
		metaJSON, _ := json.Marshal(metadatas[i])
		vectorStr := floatSliceToVector(vectors[i])
		if _, err := stmt.ExecContext(ctx, ids[i], texts[i], string(metaJSON), vectorStr); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Search 执行向量相似度搜索（余弦距离）
func (c *Client) Search(ctx context.Context, queryVector []float32, topK int, minScore float32) ([]*SearchResult, error) {
	vectorStr := floatSliceToVector(queryVector)

	query := fmt.Sprintf(`
		SELECT id, text, metadata,
			   1 - (vector <=> $1::vector) AS score
		FROM %s
		WHERE 1 - (vector <=> $1::vector) >= $2
		ORDER BY vector <=> $1::vector
		LIMIT $3`,
		c.config.TableName)

	rows, err := c.db.QueryContext(ctx, query, vectorStr, minScore, topK)
	if err != nil {
		return nil, fmt.Errorf("向量搜索失败: %w", err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		var r SearchResult
		var metaStr string
		if err := rows.Scan(&r.ID, &r.Text, &metaStr, &r.Score); err != nil {
			continue
		}
		json.Unmarshal([]byte(metaStr), &r.Metadata)
		results = append(results, &r)
	}

	return results, rows.Err()
}

// Delete 删除指定 ID 的向量记录
func (c *Client) Delete(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)",
		c.config.TableName, strings.Join(placeholders, ","))

	_, err := c.db.ExecContext(ctx, query, args...)
	return err
}

// Get 获取指定 ID 的向量记录
func (c *Client) Get(ctx context.Context, id string) (*SearchResult, error) {
	query := fmt.Sprintf("SELECT id, text, metadata FROM %s WHERE id = $1", c.config.TableName)

	var r SearchResult
	var metaStr string
	if err := c.db.QueryRowContext(ctx, query, id).Scan(&r.ID, &r.Text, &metaStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	json.Unmarshal([]byte(metaStr), &r.Metadata)
	return &r, nil
}

// Count 返回向量记录总数
func (c *Client) Count(ctx context.Context) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", c.config.TableName)
	var count int64
	err := c.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

// HealthCheck 检查连接是否正常
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.db.PingContext(ctx)
}

// Close 关闭数据库连接
func (c *Client) Close() error {
	return c.db.Close()
}

func floatSliceToVector(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ","))
}
