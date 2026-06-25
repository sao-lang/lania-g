// repository.go 实现 mongodb 集成的仓储抽象与常用访问逻辑。
package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Repository 定义 MongoDB 仓储的基础操作接口。
type Repository[T any] interface {
	Collection() *mongo.Collection
	FindOne(ctx context.Context, filter interface{}) (*T, error)
	Find(ctx context.Context, filter interface{}, opts ...*options.FindOptions) ([]T, error)
	Create(ctx context.Context, document *T) error
	Update(ctx context.Context, filter interface{}, update interface{}) error
	Delete(ctx context.Context, filter interface{}) error
	Count(ctx context.Context, filter interface{}) (int64, error)
}

// BaseRepository 提供基础 CRUD 实现。
type BaseRepository[T any] struct {
	collection *mongo.Collection
}

// NewRepository 基于数据库和集合名创建一个仓储。
func NewRepository[T any](db *mongo.Database, collectionName string) Repository[T] {
	return &BaseRepository[T]{collection: db.Collection(collectionName)}
}

// Collection 返回底层集合对象。
func (r *BaseRepository[T]) Collection() *mongo.Collection { return r.collection }

// FindOne 查询一条记录。
func (r *BaseRepository[T]) FindOne(ctx context.Context, filter interface{}) (*T, error) {
	var result T
	err := r.collection.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Find 查询多条记录。
func (r *BaseRepository[T]) Find(ctx context.Context, filter interface{}, opts ...*options.FindOptions) ([]T, error) {
	cursor, err := r.collection.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []T
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// Create 插入一条记录。
func (r *BaseRepository[T]) Create(ctx context.Context, document *T) error {
	_, err := r.collection.InsertOne(ctx, document)
	return err
}

// Update 更新一条记录。
func (r *BaseRepository[T]) Update(ctx context.Context, filter interface{}, update interface{}) error {
	_, err := r.collection.UpdateOne(ctx, filter, update)
	return err
}

// Delete 删除一条记录。
func (r *BaseRepository[T]) Delete(ctx context.Context, filter interface{}) error {
	_, err := r.collection.DeleteOne(ctx, filter)
	return err
}

// Count 统计符合条件的记录数。
func (r *BaseRepository[T]) Count(ctx context.Context, filter interface{}) (int64, error) {
	return r.collection.CountDocuments(ctx, filter)
}
