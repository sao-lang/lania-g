// repository.go 实现 orm 集成的仓储抽象与常用访问逻辑。
package orm

import "gorm.io/gorm"

// Scope 表示一个可复用的 GORM 查询片段。
type Scope func(*gorm.DB) *gorm.DB

// PageResult 表示一次分页查询的结果。
type PageResult[T any] struct {
	Records []*T `json:"records"`
	Total   int64 `json:"total"`
	Page    int `json:"page"`
	Size    int `json:"size"`
	Pages   int `json:"pages"`
}

// Repository 是围绕某个实体类型封装的通用 GORM repository。
type Repository[T any] struct {
	db *gorm.DB
}

// NewRepository 基于给定 `*gorm.DB` 创建一个泛型 repository。
func NewRepository[T any](db *gorm.DB) *Repository[T] {
	return &Repository[T]{db: db}
}

// DB 返回当前 repository 绑定的底层 GORM 查询对象。
func (r *Repository[T]) DB() *gorm.DB {
	db := r.db
	if meta := GetEntity(new(T)); meta != nil && meta.Table != "" {
		return db.Table(meta.Table).Model(new(T))
	}
	return db.Model(new(T))
}

// Create 创建一条实体记录。
func (r *Repository[T]) Create(entity *T) error { return r.db.Create(entity).Error }

// CreateInBatches 分批创建多条实体记录。
func (r *Repository[T]) CreateInBatches(entities []*T, batchSize int) error {
	return r.db.CreateInBatches(entities, batchSize).Error
}

// First 按主键查询第一条匹配记录。
func (r *Repository[T]) First(id interface{}) (*T, error) {
	var entity T
	if err := r.db.First(&entity, id).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

// FirstOrCreate 查询首条匹配记录；不存在时按给定实体创建。
func (r *Repository[T]) FirstOrCreate(entity *T, attrs ...interface{}) error {
	return r.db.FirstOrCreate(entity, attrs...).Error
}

// Find 按条件查询多条实体记录。
func (r *Repository[T]) Find(query interface{}, args ...interface{}) ([]*T, error) {
	var entities []*T
	if err := r.db.Where(query, args...).Find(&entities).Error; err != nil {
		return nil, err
	}
	return entities, nil
}

// FindAll 查询当前实体的全部记录。
func (r *Repository[T]) FindAll() ([]*T, error) {
	var entities []*T
	if err := r.db.Find(&entities).Error; err != nil {
		return nil, err
	}
	return entities, nil
}

// Last 查询最后一条匹配记录。
func (r *Repository[T]) Last() (*T, error) {
	var entity T
	if err := r.db.Last(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

// Take 查询一条任意匹配记录。
func (r *Repository[T]) Take() (*T, error) {
	var entity T
	if err := r.db.Take(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

// Update 保存整个实体。
func (r *Repository[T]) Update(entity *T) error { return r.db.Save(entity).Error }

// Updates 按模型更新实体字段。
func (r *Repository[T]) Updates(entity *T) error { return r.db.Model(entity).Updates(entity).Error }

// UpdateColumn 按主键更新单个字段。
func (r *Repository[T]) UpdateColumn(column string, value interface{}, id interface{}) error {
	return r.db.Model(new(T)).Where("id = ?", id).Update(column, value).Error
}

// UpdateColumns 按主键批量更新多个字段。
func (r *Repository[T]) UpdateColumns(values map[string]interface{}, id interface{}) error {
	return r.db.Model(new(T)).Where("id = ?", id).Updates(values).Error
}

// Delete 按主键删除一条记录。
func (r *Repository[T]) Delete(id interface{}) error {
	var entity T
	return r.db.Delete(&entity, id).Error
}

// DeleteByCondition 按条件删除记录。
func (r *Repository[T]) DeleteByCondition(query interface{}, args ...interface{}) error {
	return r.db.Where(query, args...).Delete(new(T)).Error
}

// Where 追加 `WHERE` 条件并返回新的 repository。
func (r *Repository[T]) Where(query interface{}, args ...interface{}) *Repository[T] {
	return &Repository[T]{db: r.db.Where(query, args...)}
}

// Or 追加 `OR` 条件并返回新的 repository。
func (r *Repository[T]) Or(query interface{}, args ...interface{}) *Repository[T] {
	return &Repository[T]{db: r.db.Or(query, args...)}
}

// Not 追加 `NOT` 条件并返回新的 repository。
func (r *Repository[T]) Not(query interface{}, args ...interface{}) *Repository[T] {
	return &Repository[T]{db: r.db.Not(query, args...)}
}

// Select 指定查询字段并返回新的 repository。
func (r *Repository[T]) Select(query interface{}, args ...interface{}) *Repository[T] {
	return &Repository[T]{db: r.db.Select(query, args...)}
}

// Limit 设置结果数量上限并返回新的 repository。
func (r *Repository[T]) Limit(limit int) *Repository[T] {
	return &Repository[T]{db: r.db.Limit(limit)}
}

// Offset 设置结果偏移量并返回新的 repository。
func (r *Repository[T]) Offset(offset int) *Repository[T] {
	return &Repository[T]{db: r.db.Offset(offset)}
}

// Order 设置排序条件并返回新的 repository。
func (r *Repository[T]) Order(value interface{}) *Repository[T] {
	return &Repository[T]{db: r.db.Order(value)}
}

// Group 设置分组条件并返回新的 repository。
func (r *Repository[T]) Group(name string) *Repository[T] {
	return &Repository[T]{db: r.db.Group(name)}
}

// Having 追加 `HAVING` 条件并返回新的 repository。
func (r *Repository[T]) Having(query interface{}, args ...interface{}) *Repository[T] {
	return &Repository[T]{db: r.db.Having(query, args...)}
}

// Joins 追加关联查询并返回新的 repository。
func (r *Repository[T]) Joins(query string, args ...interface{}) *Repository[T] {
	return &Repository[T]{db: r.db.Joins(query, args...)}
}

// Preload 配置预加载关联并返回新的 repository。
func (r *Repository[T]) Preload(query string, args ...interface{}) *Repository[T] {
	return &Repository[T]{db: r.db.Preload(query, args...)}
}

// Scopes 追加一组可复用查询作用域并返回新的 repository。
func (r *Repository[T]) Scopes(scopes ...Scope) *Repository[T] {
	if len(scopes) == 0 {
		return r
	}
	items := make([]func(*gorm.DB) *gorm.DB, 0, len(scopes))
	for _, scope := range scopes {
		if scope != nil {
			items = append(items, scope)
		}
	}
	return &Repository[T]{db: r.db.Scopes(items...)}
}

// Count 返回当前实体总数。
func (r *Repository[T]) Count() (int64, error) {
	var count int64
	err := r.db.Model(new(T)).Count(&count).Error
	return count, err
}

// CountByCondition 返回满足条件的记录数。
func (r *Repository[T]) CountByCondition(query interface{}, args ...interface{}) (int64, error) {
	var count int64
	err := r.db.Model(new(T)).Where(query, args...).Count(&count).Error
	return count, err
}

// Paginate 按页码和页大小查询分页结果。
func (r *Repository[T]) Paginate(page, pageSize int) (*PageResult[T], error) {
	var total int64
	var records []*T
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	db := r.db.Model(new(T))
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}
	if err := db.Offset(offset).Limit(pageSize).Find(&records).Error; err != nil {
		return nil, err
	}
	pages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		pages++
	}
	return &PageResult[T]{Records: records, Total: total, Page: page, Size: pageSize, Pages: pages}, nil
}

// Exists 判断是否存在满足条件的记录。
func (r *Repository[T]) Exists(query interface{}, args ...interface{}) (bool, error) {
	var count int64
	if err := r.db.Model(new(T)).Where(query, args...).Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// Sum 计算某列的求和结果。
func (r *Repository[T]) Sum(column string) (float64, error) {
	var sum float64
	err := r.db.Model(new(T)).Select("COALESCE(SUM(" + column + "), 0) as total").Scan(&sum).Error
	return sum, err
}

// Avg 计算某列的平均值。
func (r *Repository[T]) Avg(column string) (float64, error) {
	var avg float64
	err := r.db.Model(new(T)).Select("COALESCE(AVG(" + column + "), 0) as avg").Scan(&avg).Error
	return avg, err
}

// Max 计算某列的最大值。
func (r *Repository[T]) Max(column string) (float64, error) {
	var max float64
	err := r.db.Model(new(T)).Select("COALESCE(MAX(" + column + "), 0) as max").Scan(&max).Error
	return max, err
}

// Min 计算某列的最小值。
func (r *Repository[T]) Min(column string) (float64, error) {
	var min float64
	err := r.db.Model(new(T)).Select("COALESCE(MIN(" + column + "), 0) as min").Scan(&min).Error
	return min, err
}

// UpdateInBatches 按一组主键批量更新字段。
func (r *Repository[T]) UpdateInBatches(ids []interface{}, values map[string]interface{}) error {
	return r.db.Model(new(T)).Where("id IN ?", ids).Updates(values).Error
}

// DeleteInBatches 按一组主键批量删除记录。
func (r *Repository[T]) DeleteInBatches(ids []interface{}) error {
	return r.db.Where("id IN ?", ids).Delete(new(T)).Error
}

// Transaction 在事务中执行一组 repository 操作。
func (r *Repository[T]) Transaction(fn func(tx *Repository[T]) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(NewRepository[T](tx))
	})
}

// Raw 基于原始 SQL 创建一个新的 repository 查询上下文。
func (r *Repository[T]) Raw(sql string, values ...interface{}) *Repository[T] {
	return &Repository[T]{db: r.db.Raw(sql, values...)}
}

// Exec 执行一条不返回结果集的原始 SQL。
func (r *Repository[T]) Exec(sql string, values ...interface{}) error {
	return r.db.Exec(sql, values...).Error
}

// Pluck 抽取某一列到目标切片中。
func (r *Repository[T]) Pluck(column string, dest interface{}) error {
	return r.db.Model(new(T)).Pluck(column, dest).Error
}

// Scan 把当前查询结果扫描到目标对象中。
func (r *Repository[T]) Scan(dest interface{}) error {
	return r.db.Scan(dest).Error
}

// Model 返回当前实体类型对应的底层 GORM model 查询对象。
func (r *Repository[T]) Model() *gorm.DB {
	return r.db.Model(new(T))
}

// Unscoped 返回一个忽略软删除条件的 repository。
func (r *Repository[T]) Unscoped() *Repository[T] {
	return &Repository[T]{db: r.db.Unscoped()}
}
