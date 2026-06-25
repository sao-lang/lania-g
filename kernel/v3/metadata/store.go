package metadata

import (
	"maps"
	"reflect"
	"sync"
)

// MetadataKey 表示元数据项的键名。
type MetadataKey string

// MetadataStore 是一个线程安全的 target -> metadata 映射表。
//
// 它使用二级 map 存储：
// - 第一层按 target 的唯一标识分组
// - 第二层存放该 target 对应的 `MetadataKey -> value`
type MetadataStore struct {
	data map[interface{}]map[MetadataKey]interface{}
	mu   sync.RWMutex
}

var (
	globalStore = NewMetadataStore()
)

// NewMetadataStore 创建新的元数据存储。
//
// store 的 key 是“target 的唯一标识”（见 getKey），value 是 target -> (MetadataKey -> any) 的二级 map。
func NewMetadataStore() *MetadataStore {
	return &MetadataStore{
		data: make(map[interface{}]map[MetadataKey]interface{}),
	}
}

// GetStore 获取全局元数据存储（单例）。
//
// 当调用方不需要自己维护独立 store 时，可以直接复用这份全局实例。
func GetStore() *MetadataStore {
	return globalStore
}

// Set 设置某个 target 的元数据（线程安全）。
//
// target 可以是：
// - 函数/方法（reflect.Value.Pointer 作为唯一标识）
// - 指针对象（地址作为唯一标识）
// - 其他可比较对象（直接作为 key）
func (s *MetadataStore) Set(target interface{}, key MetadataKey, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := getKey(target)
	if s.data[k] == nil {
		s.data[k] = make(map[MetadataKey]interface{})
	}
	s.data[k][key] = value
}

// Get 获取某个 target 的某个 key 对应的元数据（线程安全）。
func (s *MetadataStore) Get(target interface{}, key MetadataKey) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := getKey(target)
	if meta, ok := s.data[k]; ok {
		val, ok := meta[key]
		return val, ok
	}
	return nil, false
}

// GetAll 获取某个 target 的全部元数据（快照）。
//
// 注意：返回的新 map 是浅拷贝，value 若是引用类型仍与内部共享。
func (s *MetadataStore) GetAll(target interface{}) map[MetadataKey]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := getKey(target)
	result := make(map[MetadataKey]interface{}, len(s.data[k]))
	maps.Copy(result, s.data[k])
	return result
}

// Has 判断某个 target 是否存在指定 key 的元数据。
func (s *MetadataStore) Has(target interface{}, key MetadataKey) bool {
	_, ok := s.Get(target, key)
	return ok
}

// getKey 将任意 target 转为稳定的 map key。
//
// - func/ptr：使用运行时地址（Pointer）作为 key
// - 其他：直接使用 target 本身作为 key
//
// 注意：使用地址作为 key 的前提是 target 的生命周期覆盖 store 的使用周期；
// 对象复用/重用场景需要调用方自行保证不发生“地址复用导致串数据”。
func getKey(target interface{}) interface{} {
	val := reflect.ValueOf(target)
	switch val.Kind() {
	case reflect.Func:
		return val.Pointer()
	case reflect.Ptr:
		return val.Pointer()
	default:
		return target
	}
}

// Scope 表示与元数据一起存放或传递的依赖注入作用域语义。
type Scope string

const (
	// ScopeSingleton 表示整个应用共享一个实例。
	ScopeSingleton Scope = "singleton"
	// ScopeRequest 表示每次请求上下文创建一个实例。
	ScopeRequest Scope = "request"
	// ScopeTransient 表示每次解析都创建新实例。
	ScopeTransient Scope = "transient"
)
