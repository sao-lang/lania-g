// model.go 定义 mongodb 集成使用的数据模型与元信息结构。
package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Model 定义具备主键读写能力的 Mongo 模型接口。
type Model interface {
	GetID() primitive.ObjectID
	SetID(id primitive.ObjectID)
}

// BaseModel 提供一个带 `_id` 字段的基础模型实现。
type BaseModel struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
}

// GetID 返回模型 ID。
func (m *BaseModel) GetID() primitive.ObjectID { return m.ID }

// SetID 设置模型 ID。
func (m *BaseModel) SetID(id primitive.ObjectID) { m.ID = id }

// BeforeCreate 在创建前为零值 ID 自动生成一个 ObjectID。
func (m *BaseModel) BeforeCreate() {
	if m.ID.IsZero() {
		m.ID = primitive.NewObjectID()
	}
}

// FindByID 按 `_id` 查询一条模型记录。
func FindByID[T Model](ctx context.Context, collection *mongo.Collection, id primitive.ObjectID) (*T, error) {
	var result T
	err := collection.FindOne(ctx, bson.M{"_id": id}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateByID 按 `_id` 更新一条记录。
func UpdateByID[T Model](ctx context.Context, collection *mongo.Collection, id primitive.ObjectID, update interface{}) error {
	_, err := collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

// DeleteByID 按 `_id` 删除一条记录。
func DeleteByID(ctx context.Context, collection *mongo.Collection, id primitive.ObjectID) error {
	_, err := collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
