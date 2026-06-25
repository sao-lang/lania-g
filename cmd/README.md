# cmd 示例说明

`cmd/` 下的示例用于展示多协议能力、调试方式和框架扩展场景。

请按以下方式理解这些示例：

- `cmd/http-app-demo` 是业务接入示例，推荐作为最小 HTTP 应用骨架参考
- `cmd/graphql-app-demo` 是业务接入示例，推荐作为最小 GraphQL 应用骨架参考
- `cmd/ws-app-demo` 是业务接入示例，推荐作为最小 WebSocket 应用骨架参考
- `cmd/grpc-app-demo` 是业务接入示例，推荐作为最小 gRPC 应用骨架参考；当前 unary 和 streaming 也统一采用 `CompositeStruct` 风格
- `cmd/*-demo` 主要是能力演示与高级示例
- 这些示例可能会直接使用部分内部基础设施包，以便展示框架机制
- 业务应用初始化时，不建议直接把这些 demo 作为生产骨架复制

业务接入推荐顺序：

1. 先阅读 `../docs/标准接入模板.md`
2. 运行或参考 `cmd/http-app-demo` / `cmd/graphql-app-demo` / `cmd/ws-app-demo` / `cmd/grpc-app-demo`
3. 使用 `application.NewWithOptions(...)` + `application.NewRegistry()`
4. 通过 mounted adapter 的 `adapter.API()` 注册协议声明
5. 将 `cmd/*-demo` 作为能力参考，而不是边界规范

## gRPC Demo 调试

`cmd/grpc-demo` 已开启 gRPC reflection，可直接使用 `grpcurl` 调试。

启动方式：

```bash
go run ./cmd/grpc-demo
```

列出服务：

```bash
grpcurl -plaintext localhost:50051 list
```

查看某个服务的方法：

```bash
grpcurl -plaintext localhost:50051 describe StreamService
```

### Unary

`grpc-demo` 里的 unary handler 目前默认采用 `CompositeStruct` 写法，用 `req:"true"` / `header:"..."` 把请求消息和 metadata 收口进一个参数 DTO。

注册账号：

```bash
grpcurl -plaintext \
  -d '{"username":"demo","password":"123456"}' \
  localhost:50051 AuthService/Register
```

登录拿 token：

```bash
grpcurl -plaintext \
  -d '{"username":"demo","password":"123456"}' \
  localhost:50051 AuthService/Login
```

带认证读取用户列表：

```bash
grpcurl -plaintext \
  -H 'Authorization: Bearer <token>' \
  -H 'X-Trace-Id: trace-demo-001' \
  -d '{"keyword":"","page":1,"size":10}' \
  localhost:50051 UserService/List
```

### Server Streaming

`grpc-demo` 里的 streaming handler 现在也支持 `CompositeStruct`，示例代码默认把首请求消息和 stream wrapper 收口到一个 DTO。

服务端连续推送用户事件：

```bash
grpcurl -plaintext \
  -d '{"keyword":"","count":3}' \
  localhost:50051 StreamService/WatchUsers
```

### Client Streaming

客户端连续上传多条用户数据，服务端在结束时返回汇总结果：

```bash
cat <<'EOF' | grpcurl -plaintext -d @ localhost:50051 StreamService/UploadUsers
{"name":"Tom","email":"tom@example.com","age":18}
{"name":"Jerry","email":"jerry@example.com","age":20}
EOF
```

### Bidirectional Streaming

双向流聊天示例：

```bash
cat <<'EOF' | grpcurl -plaintext -d @ localhost:50051 StreamService/ChatUsers
{"text":"hello"}
{"text":"lania"}
EOF
```

说明：

- `AuthService/*` 和 `StreamService/*` 为公开演示接口，不需要额外认证
- `UserService/*` 需要先通过 `AuthService/Login` 获取 token，再通过 `Authorization` header 传入
- `grpc-demo` 使用的是 `google.protobuf.Struct` 作为消息体，因此 `grpcurl` 可以直接用 JSON 传参
