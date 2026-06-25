// tx.go 实现 orm 集成的事务上下文与事务辅助能力。
package orm

import (
	"context"
	"reflect"

	"gorm.io/gorm"
)

type txContextKey struct{}

// TransactionManager 定义事务执行、提取和透传当前 `*gorm.DB` 的能力。
type TransactionManager interface {
	Do(ctx context.Context, fn func(context.Context, *gorm.DB) error) error
	Current(ctx context.Context) *gorm.DB
	With(ctx context.Context, db *gorm.DB) context.Context
}

type transactionManager struct {
	defaultDB *gorm.DB
}

// NewTransactionManager 基于默认 datasource 创建一个事务管理器。
func NewTransactionManager(db *gorm.DB) TransactionManager {
	return &transactionManager{defaultDB: db}
}

// Do 在事务中执行回调，并把当前事务连接写入上下文。
func (m *transactionManager) Do(ctx context.Context, fn func(context.Context, *gorm.DB) error) error {
	return m.defaultDB.Transaction(func(tx *gorm.DB) error {
		return fn(m.With(ctx, tx), tx)
	})
}

// Current 返回上下文中的当前事务连接；若不存在则回退到默认 datasource。
func (m *transactionManager) Current(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return m.defaultDB
	}
	if tx, ok := ctx.Value(txContextKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return m.defaultDB
}

// With 把事务连接写入上下文，供后续逻辑复用。
func (m *transactionManager) With(ctx context.Context, db *gorm.DB) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, txContextKey{}, db)
}

// UnitOfWork 是基于 TransactionManager 封装的轻量事务入口。
type UnitOfWork struct {
	manager TransactionManager
}

// NewUnitOfWork 创建一个事务工作单元。
func NewUnitOfWork(manager TransactionManager) *UnitOfWork {
	return &UnitOfWork{manager: manager}
}

// Do 在事务工作单元中执行回调。
func (u *UnitOfWork) Do(ctx context.Context, fn func(context.Context) error) error {
	return u.manager.Do(ctx, func(txCtx context.Context, _ *gorm.DB) error {
		return fn(txCtx)
	})
}

// DBFromContext 从上下文提取当前事务连接，不存在时回退到给定 datasource。
func DBFromContext(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if ctx == nil {
		return fallback
	}
	if tx, ok := ctx.Value(txContextKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return fallback
}

// TransactionManagerToken 返回事务管理器对应的 DI token。
func TransactionManagerToken() reflect.Type {
	return reflect.TypeFor[TransactionManager]()
}
