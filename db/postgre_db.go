package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pylemonorg/gotools/logger"

	_ "github.com/lib/pq" // PostgreSQL 驱动
)

// PostgreSQL 相关的哨兵错误。
var (
	ErrPgNilParams = errors.New("postgres: 连接参数不能为 nil")
	ErrPgNotInit   = errors.New("postgres: 连接未初始化")
)

// maxBatchErrors 批量操作中最多记录的错误数，防止内存膨胀。
const maxBatchErrors = 10

// PostgresClient 封装了 database/sql 的 PostgreSQL 连接，提供便捷的 CRUD 操作。
type PostgresClient struct {
	db     *sql.DB
	params *PostgresParams
}

// PostgresParams 定义 PostgreSQL 连接所需的参数。
type PostgresParams struct {
	Host     string // 主机地址
	Port     int    // 端口号
	User     string // 用户名
	Password string // 密码
	DBName   string // 数据库名
	SSLMode  string // SSL 模式，为空时默认 "disable"
}

// sslModeOrDefault 返回 SSLMode 值，为空时返回 "disable"。
func (p *PostgresParams) sslModeOrDefault() string {
	if strings.TrimSpace(p.SSLMode) == "" {
		return "disable"
	}
	return p.SSLMode
}

// dsn 构建 PostgreSQL 连接字符串。
func (p *PostgresParams) dsn() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.User, p.Password, p.DBName, p.sslModeOrDefault())
}

// dsnWithDB 构建连接到指定数据库的连接字符串。
func (p *PostgresParams) dsnWithDB(dbname string) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.User, p.Password, dbname, p.sslModeOrDefault())
}

// validatePostgresParams 校验 PostgreSQL 连接参数的必填项。
func validatePostgresParams(p *PostgresParams) error {
	var missing []string
	if strings.TrimSpace(p.Host) == "" {
		missing = append(missing, "Host")
	}
	if p.Port <= 0 {
		missing = append(missing, "Port")
	}
	if strings.TrimSpace(p.User) == "" {
		missing = append(missing, "User")
	}
	if strings.TrimSpace(p.DBName) == "" {
		missing = append(missing, "DBName")
	}
	if len(missing) > 0 {
		return fmt.Errorf("postgres: 缺少必要连接参数: %s", strings.Join(missing, ", "))
	}
	return nil
}

// NewPostgresClient 根据给定参数创建 PostgresClient 实例并测试连通性。
func NewPostgresClient(params *PostgresParams) (*PostgresClient, error) {
	if params == nil {
		return nil, ErrPgNilParams
	}
	if err := validatePostgresParams(params); err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", params.dsn())
	if err != nil {
		return nil, fmt.Errorf("postgres: 打开连接失败: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)

	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres: 连接测试失败: %w", err)
	}

	logger.Infof("postgres: 连接成功 %s:%d/%s", params.Host, params.Port, params.DBName)
	return &PostgresClient{db: db, params: params}, nil
}

// GetDB 返回底层 *sql.DB，可用于执行未封装的高级操作。
func (c *PostgresClient) GetDB() *sql.DB { return c.db }

// Close 关闭数据库连接。
func (c *PostgresClient) Close() error {
	if c.db == nil {
		return nil
	}
	if err := c.db.Close(); err != nil {
		logger.Warnf("postgres: 关闭连接失败: %v", err)
		return err
	}
	logger.Infof("postgres: 连接已关闭")
	return nil
}

// ---------------------------------------------------------------------------
// 数据库管理
// ---------------------------------------------------------------------------

// EnsureDatabaseExists 确保目标数据库存在，不存在则自动创建。
// 通过连接默认的 postgres 数据库执行管理操作，params.DBName 为目标数据库名。
func EnsureDatabaseExists(params *PostgresParams) error {
	if params == nil {
		return ErrPgNilParams
	}

	conn, err := sql.Open("postgres", params.dsnWithDB("postgres"))
	if err != nil {
		return fmt.Errorf("postgres: 连接默认数据库失败: %w", err)
	}
	defer conn.Close()

	if err = conn.Ping(); err != nil {
		return fmt.Errorf("postgres: ping 默认数据库失败: %w", err)
	}

	var exists bool
	if err = conn.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", params.DBName).Scan(&exists); err != nil {
		return fmt.Errorf("postgres: 查询数据库是否存在失败: %w", err)
	}
	if exists {
		logger.Infof("postgres: 数据库 [%s] 已存在", params.DBName)
		return nil
	}

	// CREATE DATABASE 不支持参数化查询，此处拼接安全可控（值来自配置）
	if _, err = conn.Exec(fmt.Sprintf("CREATE DATABASE %s", params.DBName)); err != nil {
		return fmt.Errorf("postgres: 创建数据库 [%s] 失败: %w", params.DBName, err)
	}

	logger.Infof("postgres: 数据库 [%s] 创建成功", params.DBName)
	return nil
}

// ExistsTable 检查 public schema 下指定表是否存在。
func (c *PostgresClient) ExistsTable(tableName string) (bool, error) {
	if c.db == nil {
		return false, ErrPgNotInit
	}
	const query = `SELECT EXISTS(SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = $1)`
	var exists bool
	if err := c.db.QueryRow(query, tableName).Scan(&exists); err != nil {
		return false, fmt.Errorf("postgres: 查询表 [%s] 是否存在失败: %w", tableName, err)
	}
	return exists, nil
}

// ---------------------------------------------------------------------------
// CRUD 操作
// ---------------------------------------------------------------------------

// Insert 执行插入语句，自动追加 RETURNING id 尝试获取自增主键。
// 若表无 id 列会回退到普通 Exec。
func (c *PostgresClient) Insert(query string, args ...any) (int64, error) {
	if c.db == nil {
		return 0, ErrPgNotInit
	}

	var lastInsertID int64
	err := c.db.QueryRow(query+" RETURNING id", args...).Scan(&lastInsertID)
	if err == nil {
		return lastInsertID, nil
	}

	// RETURNING id 失败，回退到普通插入
	result, execErr := c.db.Exec(query, args...)
	if execErr != nil {
		return 0, fmt.Errorf("postgres: 插入失败: %w", execErr)
	}
	lastInsertID, _ = result.LastInsertId()
	return lastInsertID, nil
}

// InsertWithReturning 执行包含 RETURNING 子句的插入语句，将返回值扫描到 dest。
func (c *PostgresClient) InsertWithReturning(query string, dest any, args ...any) error {
	if c.db == nil {
		return ErrPgNotInit
	}
	if err := c.db.QueryRow(query, args...).Scan(dest); err != nil {
		return fmt.Errorf("postgres: 插入失败: %w", err)
	}
	return nil
}

// Query 执行查询，返回多行结果。调用方需负责关闭 *sql.Rows。
func (c *PostgresClient) Query(query string, args ...any) (*sql.Rows, error) {
	if c.db == nil {
		return nil, ErrPgNotInit
	}
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: 查询失败: %w", err)
	}
	return rows, nil
}

// QueryRow 执行查询，返回单行结果。
func (c *PostgresClient) QueryRow(query string, args ...any) *sql.Row {
	if c.db == nil {
		return nil
	}
	return c.db.QueryRow(query, args...)
}

// QueryOne 执行查询并将单行结果扫描到 dest，无数据时返回 sql.ErrNoRows。
func (c *PostgresClient) QueryOne(query string, dest any, args ...any) error {
	if c.db == nil {
		return ErrPgNotInit
	}
	if err := c.db.QueryRow(query, args...).Scan(dest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("postgres: 查询失败: %w", err)
	}
	return nil
}

// Exec 执行非查询 SQL（INSERT / UPDATE / DELETE 等）。
func (c *PostgresClient) Exec(query string, args ...any) (sql.Result, error) {
	if c.db == nil {
		return nil, ErrPgNotInit
	}
	result, err := c.db.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: 执行 SQL 失败: %w", err)
	}
	return result, nil
}

// Update 执行更新语句，返回受影响的行数。
func (c *PostgresClient) Update(query string, args ...any) (int64, error) {
	result, err := c.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("postgres: 获取受影响行数失败: %w", err)
	}
	return n, nil
}

// Delete 执行删除语句，返回受影响的行数。
func (c *PostgresClient) Delete(query string, args ...any) (int64, error) {
	return c.Update(query, args...)
}

// ---------------------------------------------------------------------------
// 事务操作
// ---------------------------------------------------------------------------

// BeginTx 开启一个事务。
func (c *PostgresClient) BeginTx() (*sql.Tx, error) {
	if c.db == nil {
		return nil, ErrPgNotInit
	}
	if err := c.db.Ping(); err != nil {
		return nil, fmt.Errorf("postgres: 连接无效: %w", err)
	}
	tx, err := c.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("postgres: 开始事务失败: %w", err)
	}
	return tx, nil
}

// ---------------------------------------------------------------------------
// 批量插入
// ---------------------------------------------------------------------------

// BatchInsertResult 描述批量插入的执行结果。
type BatchInsertResult struct {
	SuccessCount int64   // 成功插入的行数
	FailedCount  int64   // 失败的行数
	Errors       []error // 错误列表（最多记录 maxBatchErrors 条）
}

// BatchInsert 在单个事务中批量插入数据（严格模式）。
// 任意一条失败则整个事务回滚，所有数据都不会插入。
func (c *PostgresClient) BatchInsert(query string, dataList [][]any) (int64, error) {
	if c.db == nil {
		return 0, ErrPgNotInit
	}

	tx, err := c.BeginTx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(query)
	if err != nil {
		return 0, fmt.Errorf("postgres: 准备语句失败: %w", err)
	}
	defer stmt.Close()

	var totalRows int64
	for i, args := range dataList {
		result, err := stmt.Exec(args...)
		if err != nil {
			return 0, fmt.Errorf("postgres: 第 %d 条数据插入失败: %w", i+1, err)
		}
		n, _ := result.RowsAffected()
		totalRows += n
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("postgres: 提交事务失败: %w", err)
	}
	return totalRows, nil
}

// BatchInsertTolerant 逐条插入数据（容错模式，无事务）。
// 单条失败不影响其他条目，最终返回成功/失败统计。
func (c *PostgresClient) BatchInsertTolerant(query string, dataList [][]any) (*BatchInsertResult, error) {
	if c.db == nil {
		return nil, ErrPgNotInit
	}

	res := &BatchInsertResult{}

	stmt, err := c.db.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("postgres: 准备语句失败: %w", err)
	}
	defer stmt.Close()

	for i, args := range dataList {
		execResult, err := stmt.Exec(args...)
		if err != nil {
			res.FailedCount++
			if len(res.Errors) < maxBatchErrors {
				res.Errors = append(res.Errors, fmt.Errorf("第 %d 条: %w", i+1, err))
			}
			continue
		}
		n, _ := execResult.RowsAffected()
		res.SuccessCount += n
	}

	if res.SuccessCount == 0 && res.FailedCount > 0 {
		return res, fmt.Errorf("postgres: 全部 %d 条数据插入失败", res.FailedCount)
	}
	return res, nil
}

// BatchInsertTolerantWithTx 分批事务插入数据（容错模式）。
// 将 dataList 按 batchSize 分批，每批使用独立事务；
// 单批失败不影响其他批次。batchSize <= 0 时默认 100。
func (c *PostgresClient) BatchInsertTolerantWithTx(query string, dataList [][]any, batchSize int) (*BatchInsertResult, error) {
	if c.db == nil {
		return nil, ErrPgNotInit
	}
	if batchSize <= 0 {
		batchSize = 100
	}

	res := &BatchInsertResult{}
	totalBatches := (len(dataList) + batchSize - 1) / batchSize

	for batchIdx := 0; batchIdx < totalBatches; batchIdx++ {
		start := batchIdx * batchSize
		end := start + batchSize
		if end > len(dataList) {
			end = len(dataList)
		}
		batchData := dataList[start:end]

		batchRows, batchFails, err := c.execBatch(query, batchData, batchIdx+1)
		if err != nil {
			// 整批失败
			res.FailedCount += int64(len(batchData))
			if len(res.Errors) < maxBatchErrors {
				res.Errors = append(res.Errors, err)
			}
			continue
		}
		res.SuccessCount += batchRows
		res.FailedCount += batchFails
	}

	if res.SuccessCount == 0 && res.FailedCount > 0 {
		return res, fmt.Errorf("postgres: 全部 %d 条数据插入失败", res.FailedCount)
	}
	return res, nil
}

// execBatch 在独立事务中执行一批插入，返回成功行数、失败条数和致命错误。
func (c *PostgresClient) execBatch(query string, batchData [][]any, batchNum int) (successRows, failCount int64, fatalErr error) {
	tx, err := c.BeginTx()
	if err != nil {
		return 0, 0, fmt.Errorf("批次 %d 开始事务失败: %w", batchNum, err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(query)
	if err != nil {
		return 0, 0, fmt.Errorf("批次 %d 准备语句失败: %w", batchNum, err)
	}
	defer stmt.Close()

	for i, args := range batchData {
		execResult, err := stmt.Exec(args...)
		if err != nil {
			// 死锁导致事务不可用，整批回滚
			if strings.Contains(err.Error(), "deadlock") {
				return 0, 0, fmt.Errorf("批次 %d 第 %d 条死锁，批次已回滚: %w", batchNum, i+1, err)
			}
			failCount++
			logger.Warnf("postgres: 批次 %d 第 %d 条插入失败: %v", batchNum, i+1, err)
			continue
		}
		n, _ := execResult.RowsAffected()
		successRows += n
	}

	if err = tx.Commit(); err != nil {
		return 0, int64(len(batchData)), fmt.Errorf("批次 %d 提交失败: %w", batchNum, err)
	}
	return successRows, failCount, nil
}
