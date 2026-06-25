# lania-g v3 MongoDB 接入

## 1. 提供能力

`integrations/mongodb` 作为独立 integration 提供：

- `ForRoot`
- `Factory`
- `InjectDatabase`
- `InjectClient`
- `DatabaseRef[T]`

## 2. 基本接入

```go
mongoModule, err := mongodb.ForRoot(mongodb.Config{
	Name:     "analytics",
	URI:      "mongodb://127.0.0.1:27017",
	Database: "demo",
})
if err != nil {
	panic(err)
}
```

## 3. 参数注入

- 默认数据库
  - `mongodb.InjectDatabase`
- 客户端
  - `mongodb.InjectClient`
- 命名数据库
  - `mongodb.DatabaseRef[T]`

## 4. 设计约定

- 保持独立 integration，不并入 `integrations/orm`
- 继续遵循 `ForRoot / Factory / InjectXxx / XxxRef[T]` 模式

## 5. 高级特性

### 5.1 Repository 模式

现在支持 Repository 模式，提供基本的 CRUD 操作：

```go
// 定义模型
type User struct {
    mongodb.BaseModel
    Name  string `bson:"name" json:"name"`
    Email string `bson:"email" json:"email"`
    Age   int    `bson:"age" json:"age"`
}

// 创建 Repository
func NewUserRepository(db *mongo.Database) mongodb.Repository[User] {
    return mongodb.NewRepository[User](db, "users")
}

// 使用 Repository
func (r *UserRepository) CreateUser(ctx context.Context, user *User) error {
    user.BeforeCreate() // 自动生成 ID
    return r.Create(ctx, user)
}

func (r *UserRepository) GetUser(ctx context.Context, id string) (*User, error) {
    oid, err := mongodb.ToObjectID(id)
    if err != nil {
        return nil, err
    }
    return r.FindOne(ctx, bson.M{"_id": oid})
}

func (r *UserRepository) ListUsers(ctx context.Context, filter bson.M) ([]User, error) {
    return r.Find(ctx, filter)
}

func (r *UserRepository) UpdateUser(ctx context.Context, id string, update bson.M) error {
    oid, err := mongodb.ToObjectID(id)
    if err != nil {
        return err
    }
    return r.Update(ctx, bson.M{"_id": oid}, bson.M{"$set": update})
}

func (r *UserRepository) DeleteUser(ctx context.Context, id string) error {
    oid, err := mongodb.ToObjectID(id)
    if err != nil {
        return err
    }
    return r.Delete(ctx, bson.M{"_id": oid})
}

func (r *UserRepository) CountUsers(ctx context.Context, filter bson.M) (int64, error) {
    return r.Count(ctx, filter)
}
```

### 5.2 事务辅助函数

现在支持事务和会话辅助函数：

```go
// 使用事务
err := mongodb.WithTransaction(ctx, client, func(sessionContext context.Context, session mongo.Session) error {
    // 在事务中执行操作
    _, err := collection.InsertOne(sessionContext, bson.M{"name": "John"})
    if err != nil {
        return err
    }
    _, err = collection.UpdateOne(sessionContext, bson.M{"name": "Jane"}, bson.M{"$set": bson.M{"age": 30}})
    return err
})

// 使用会话
err := mongodb.WithSession(ctx, client, func(sessionContext context.Context, session mongo.Session) error {
    // 在会话中执行操作
    _, err := collection.FindOne(sessionContext, bson.M{"name": "John"}).Decode(&result)
    return err
})
```

### 5.3 ObjectID 工具函数

现在提供 ObjectID 工具函数：

```go
// 转换字符串为 ObjectID
oid, err := mongodb.ToObjectID("609c7f7f9a9b9c1a2b3c4d5e")

// 转换字符串为 ObjectID 或 panic
oid := mongodb.MustObjectID("609c7f7f9a9b9c1a2b3c4d5e")

// 检查字符串是否为有效的 ObjectID
valid := mongodb.IsValidObjectID("609c7f7f9a9b9c1a2b3c4d5e")
```

### 5.4 Model 接口和 BaseModel

现在提供 Model 接口和 BaseModel 实现：

```go
// 定义模型
type Product struct {
    mongodb.BaseModel
    Name  string  `bson:"name" json:"name"`
    Price float64 `bson:"price" json:"price"`
    Stock int     `bson:"stock" json:"stock"`
}

// 使用 BaseModel 的方法
product := &Product{
    Name:  "Product 1",
    Price: 99.99,
    Stock: 100,
}

product.BeforeCreate() // 自动生成 ID
fmt.Println("Product ID:", product.GetID())

// 查找、更新、删除辅助函数
product, err := mongodb.FindByID[Product](ctx, collection, oid)
err := mongodb.UpdateByID[Product](ctx, collection, oid, bson.M{"$set": bson.M{"price": 89.99}})
err := mongodb.DeleteByID(ctx, collection, oid)
```

### 5.5 依赖注入示例

```go
// 控制器
 type UserController struct {
    DB         *mongo.Database        `inject:""`
    Client     *mongo.Client          `inject:""`
    UserRepo   mongodb.Repository[User] `inject:""`
 }

// 注册 Repository 到容器
func RegisterRepositories(reg *registry.Registry) {
    reg.RegisterProvider(di.ProviderFromFactory(func(db *mongo.Database) mongodb.Repository[User] {
        return mongodb.NewRepository[User](db, "users")
    }, di.Singleton))
}
```
