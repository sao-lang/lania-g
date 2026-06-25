// store_mongo.go 提供 outbox 集成的 Mongo 存储实现。
package outbox

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBStore 是基于 MongoDB 的 Outbox Store 实现。
type MongoDBStore struct {
	collection *mongo.Collection
}

// NewMongoDBStore 基于数据库和集合名创建一个 MongoDB Store。
func NewMongoDBStore(db *mongo.Database, collectionName string) Store {
	return &MongoDBStore{collection: db.Collection(collectionName)}
}

// Save 保存一条消息。
func (s *MongoDBStore) Save(ctx context.Context, msg *Message) error {
	_, err := s.collection.InsertOne(ctx, msg)
	return err
}

// Get 按 ID 读取一条消息。
func (s *MongoDBStore) Get(ctx context.Context, id string) (*Message, error) {
	var msg Message
	err := s.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// ListPending 列出待处理或失败且达到可用时间的消息。
func (s *MongoDBStore) ListPending(ctx context.Context, limit int) ([]*Message, error) {
	filter := bson.M{
		"status":       bson.M{"$in": []Status{StatusPending, StatusFailed}},
		"available_at": bson.M{"$lte": time.Now()},
	}

	options := &options.FindOptions{}
	if limit > 0 {
		limit64 := int64(limit)
		options.Limit = &limit64
	}

	cursor, err := s.collection.Find(ctx, filter, options)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var messages []*Message
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, err
	}

	return messages, nil
}

// Mark 更新消息状态、错误信息与重试次数。
func (s *MongoDBStore) Mark(ctx context.Context, id string, status Status, lastError string, attempts int) error {
	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"last_error": lastError,
			"attempts":   attempts,
			"updated_at": time.Now(),
		},
	}

	_, err := s.collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}
