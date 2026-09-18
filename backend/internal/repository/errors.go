package repository

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// 数据库唯一约束冲突识别：
//   - 生产环境 PostgreSQL 驱动返回 *pgconn.PgError，Code=23505（unique_violation）
//   - 测试环境 SQLite（modernc/glebarez）返回包含 "UNIQUE constraint failed" 的错误
//
// service 层据此将“并发下被部分唯一索引拒绝”的插入转换为业务 409 冲突，
// 事务回滚后失败方不会留下任何记录。

// ErrDuplicateKey 唯一约束冲突的仓储层哨兵错误。
var ErrDuplicateKey = errors.New("duplicate key violates unique constraint")

// pgUniqueViolation PostgreSQL unique_violation SQLSTATE。
const pgUniqueViolation = "23505"

// IsDuplicateKeyErr 判断是否为唯一约束冲突错误。
func IsDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}
	// SQLite：modernc 驱动错误信息形如 "UNIQUE constraint failed: planting_plans.plot_id"
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
