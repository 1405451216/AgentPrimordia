// Package pgvector 实现 PostgreSQL + pgvector 向量存储。
package pgvector

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 距离度量类型
type DistanceType string

const (
	CosineDistance    DistanceType = "cosine"
	L2Distance        DistanceType = "l2"
	InnerProduct      DistanceType = "inner_product"
)

// 索引类型
type IndexType string

const (
	HNSWIndex   IndexType = "hnsw"
	IVFFlatIndex IndexType = "ivfflat"
)

// Config 配置
type Config struct {
	ConnString     string        // postgres://user:pass@host:5432/dbname
	TableName      string        // 表名（默认 agent_vectors）
	Dimensions     int           // 向量维度
	Distance       DistanceType  // 距离度量
	IndexType      IndexType     // 索引类型
	MaxConnections int           // 连接池大小（默认 5）
	Timeout        time.Duration // 超时（默认 30s）
}

// VectorEntry 向量条目
type VectorEntry struct {
	ID       string
	Vector   []float32
	Text     string
	Metadata map[string]string
}

// SearchResult 搜索结果
type SearchResult struct {
	ID       string
	Distance float32
	Text     string
	Metadata map[string]string
	Score    float32
}

// Store 是 pgvector 存储实例
type Store struct {
	db        *pgxpool.Pool
	table     string
	dim       int
	distance  DistanceType
	indexType IndexType
}

// New 创建 pgvector 存储
func New(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.TableName == "" {
		cfg.TableName = "agent_vectors"
	}
	if cfg.Dimensions <= 0 {
		cfg.Dimensions = 16
	}
	if cfg.Distance == "" {
		cfg.Distance = CosineDistance
	}
	if cfg.IndexType == "" {
		cfg.IndexType = HNSWIndex
	}
	if cfg.MaxConnections <= 0 {
		cfg.MaxConnections = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.ConnString)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	poolConfig.MaxConns = int32(cfg.MaxConnections)

	db, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	store := &Store{
		db:        db,
		table:     cfg.TableName,
		dim:       cfg.Dimensions,
		distance:  cfg.Distance,
		indexType: cfg.IndexType,
	}

	if err := store.initSchema(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return store, nil
}

// initSchema 初始化数据库 schema
func (s *Store) initSchema(ctx context.Context) error {
	// 启用 pgvector 扩展
	if _, err := s.db.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector;"); err != nil {
		return fmt.Errorf("create extension: %w", err)
	}

	// 创建向量表
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			text TEXT NOT NULL DEFAULT '',
			vector vector(%d) NOT NULL,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT now()
		);
	`, s.table, s.dim)

	if _, err := s.db.Exec(ctx, query); err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	// 创建索引
	if err := s.createIndex(ctx); err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	return nil
}

// createIndex 创建向量索引
func (s *Store) createIndex(ctx context.Context) error {
	indexName := fmt.Sprintf("idx_%s_vector", s.table)

	var op string
	switch s.distance {
	case CosineDistance:
		op = vectorOpCosine()
	case L2Distance:
		op = vectorOpL2()
	case InnerProduct:
		op = vectorOpIP()
	}

	var query string
	switch s.indexType {
	case HNSWIndex:
		query = fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw (%s)",
			indexName, s.table, op,
		)
	case IVFFlatIndex:
		query = fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s USING ivfflat (%s) WITH (lists = 100)",
			indexName, s.table, op,
		)
	default:
		query = fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw (%s)",
			indexName, s.table, op,
		)
	}

	_, err := s.db.Exec(ctx, query)
	return err
}

// vectorOpCosine 返回余弦距离操作符
func vectorOpCosine() string { return "vector_cosine_ops" }

// vectorOpL2 返回 L2 距离操作符
func vectorOpL2() string { return "vector_l2_ops" }

// vectorOpIP 返回内积操作符
func vectorOpIP() string { return "vector_ip_ops" }

// Add 添加向量
func (s *Store) Add(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	return s.AddWithText(ctx, id, vector, "", metadata)
}

// AddWithText 添加向量（含文本字段）
func (s *Store) AddWithText(ctx context.Context, id string, vector []float32, text string, metadata map[string]string) error {
	if len(vector) != s.dim {
		return fmt.Errorf("dimension mismatch: expected %d, got %d", s.dim, len(vector))
	}

	vecStr := float32SliceToVectorString(vector)

	// 将 metadata map 转为 JSON
	metaJSON := "{}"
	if len(metadata) > 0 {
		parts := make([]string, 0, len(metadata))
		for k, v := range metadata {
			parts = append(parts, fmt.Sprintf("%q:%q", k, v))
		}
		metaJSON = "{" + strings.Join(parts, ",") + "}"
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (id, text, vector, metadata) VALUES ($1, $2, $3::vector, $4::jsonb) ON CONFLICT (id) DO UPDATE SET text = EXCLUDED.text, vector = EXCLUDED.vector, metadata = EXCLUDED.metadata",
		s.table,
	)

	_, err := s.db.Exec(ctx, query, id, text, vecStr, metaJSON)
	return err
}

// BatchInsert 批量插入向量记录
func (s *Store) BatchInsert(ctx context.Context, ids []string, vectors [][]float32, texts []string, metadatas []map[string]string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	query := fmt.Sprintf(
		"INSERT INTO %s (id, text, vector, metadata) VALUES ($1, $2, $3::vector, $4::jsonb) ON CONFLICT (id) DO UPDATE SET text = EXCLUDED.text, vector = EXCLUDED.vector, metadata = EXCLUDED.metadata",
		s.table,
	)

	for i := range ids {
		if len(vectors[i]) != s.dim {
			return fmt.Errorf("dimension mismatch at index %d: expected %d, got %d", i, s.dim, len(vectors[i]))
		}
		vecStr := float32SliceToVectorString(vectors[i])
		metaJSON := "{}"
		if metadatas != nil && i < len(metadatas) && len(metadatas[i]) > 0 {
			parts := make([]string, 0, len(metadatas[i]))
			for k, v := range metadatas[i] {
				parts = append(parts, fmt.Sprintf("%q:%q", k, v))
			}
			metaJSON = "{" + strings.Join(parts, ",") + "}"
		}
		text := ""
		if texts != nil && i < len(texts) {
			text = texts[i]
		}
		if _, err := tx.Exec(ctx, query, ids[i], text, vecStr, metaJSON); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// Get 获取向量
func (s *Store) Get(ctx context.Context, id string) (*VectorEntry, error) {
	query := fmt.Sprintf("SELECT id, text, vector, metadata FROM %s WHERE id = $1", s.table)
	row := s.db.QueryRow(ctx, query, id)

	var entry VectorEntry
	var vecStr string
	var metaJSON string
	if err := row.Scan(&entry.ID, &entry.Text, &vecStr, &metaJSON); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("vector %s not found", id)
		}
		return nil, err
	}

	vec, err := float32SliceToFloat32(vecStr)
	if err != nil {
		return nil, err
	}
	entry.Vector = vec

	// 简化：metadata 存为单行文本
	entry.Metadata = map[string]string{"raw": metaJSON}
	return &entry, nil
}

// Search KNN 搜索
func (s *Store) Search(ctx context.Context, query []float32, topK int, filters map[string]string) ([]SearchResult, error) {
	if len(query) != s.dim {
		return nil, fmt.Errorf("dimension mismatch: expected %d, got %d", s.dim, len(query))
	}

	vecStr := float32SliceToVectorString(query)

	// 构建 WHERE 子句
	whereClause := ""
	args := []any{vecStr, topK}
	argIdx := 3

	if len(filters) > 0 {
		conds := make([]string, 0, len(filters))
		for k, v := range filters {
			conds = append(conds, fmt.Sprintf("metadata->>$%d = $%d", argIdx, argIdx+1))
			args = append(args, k, v)
			argIdx += 2
		}
		whereClause = "WHERE " + strings.Join(conds, " AND ")
	}

	var distanceOp string
	switch s.distance {
	case CosineDistance:
		distanceOp = "<=>"
	case L2Distance:
		distanceOp = "<->"
	case InnerProduct:
		distanceOp = "<#>"
	}

	sqlQuery := fmt.Sprintf(`
		SELECT id, text, vector %s $1::vector AS distance, metadata
		FROM %s
		%s
		ORDER BY distance
		LIMIT $2
	`, distanceOp, s.table, whereClause)

	rows, err := s.db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var metaJSON string
		if err := rows.Scan(&r.ID, &r.Text, &r.Distance, &metaJSON); err != nil {
			return nil, err
		}
		r.Metadata = map[string]string{"raw": metaJSON}
		r.Score = 1.0 - r.Distance // 转为分数（越大越相关）
		results = append(results, r)
	}
	return results, rows.Err()
}

// Delete 删除向量
func (s *Store) Delete(ctx context.Context, id string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", s.table)
	_, err := s.db.Exec(ctx, query, id)
	return err
}

// Close 关闭连接池
func (s *Store) Close() {
	s.db.Close()
}

// float32SliceToFloat32 转换
func float32SliceToFloat32(s string) ([]float32, error) {
	// pgvector 返回格式: [0.1,0.2,0.3]
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return []float32{}, nil
	}
	parts := strings.Split(s, ",")
	result := make([]float32, 0, len(parts))
	for _, p := range parts {
		var v float32
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%f", &v); err != nil {
			return nil, fmt.Errorf("parse float %q: %w", p, err)
		}
		result = append(result, v)
	}
	return result, nil
}

// float32SliceToVectorString 将 []float32 转为 pgvector 文本格式
func float32SliceToVectorString(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Ensure interface
// Count 返回向量记录总数
func (s *Store) Count(ctx context.Context) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", s.table)
	var count int64
	err := s.db.QueryRow(ctx, query).Scan(&count)
	return count, err
}

// HealthCheck 检查连接是否正常
func (s *Store) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.db.Ping(ctx)
}

// Ensure interface
var _ = (*Store)(nil)
var _ = sql.ErrNoRows
