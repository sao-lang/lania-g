// objectid.go 提供 mongodb 集成的 ObjectID 转换与绑定辅助。
package mongodb

import "go.mongodb.org/mongo-driver/bson/primitive"

// ToObjectID 把十六进制字符串解析为 ObjectID。
func ToObjectID(id string) (primitive.ObjectID, error) { return primitive.ObjectIDFromHex(id) }

// MustObjectID 是 ToObjectID 的 panic 版本。
func MustObjectID(id string) primitive.ObjectID {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		panic(err)
	}
	return oid
}

// IsValidObjectID 判断字符串是否为合法的 ObjectID。
func IsValidObjectID(id string) bool {
	_, err := primitive.ObjectIDFromHex(id)
	return err == nil
}
