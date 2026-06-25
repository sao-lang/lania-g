// entity.go 定义 orm 集成面向实体建模的核心抽象。
package orm

import (
	"reflect"
	"sync"
)

// EntityMetadata 描述实体类型对应的表名与主键信息。
type EntityMetadata struct {
	Table      string
	PrimaryKey string
}

var (
	entityStore   = make(map[reflect.Type]*EntityMetadata)
	entityStoreMu sync.RWMutex
)

// Entity 为某个实体类型注册表名与主键信息。
func Entity(table string, primaryKeyOrEntity interface{}, entity ...interface{}) {
	var target interface{}
	primaryKey := "ID"
	if len(entity) > 0 {
		if pk, ok := primaryKeyOrEntity.(string); ok && pk != "" {
			primaryKey = pk
		}
		target = entity[0]
	} else {
		target = primaryKeyOrEntity
	}
	if target == nil {
		return
	}
	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	entityStoreMu.Lock()
	entityStore[t] = &EntityMetadata{Table: table, PrimaryKey: primaryKey}
	entityStoreMu.Unlock()
}

// GetEntity 返回某个实体类型已注册的元数据。
func GetEntity(target interface{}) *EntityMetadata {
	if target == nil {
		return nil
	}
	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	entityStoreMu.RLock()
	defer entityStoreMu.RUnlock()
	return entityStore[t]
}
