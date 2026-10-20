---
title: 质量工具链
weight: 4
prev: /docs/internals/diagnostics
next: /docs/best-practices
---

框架用五个自研工具把「文档失真、API 漂移、模块边界腐化、性能回退」变成可执行的门禁。
它们都放在独立模块 `github.com/sao-lang/lania-g/tools` 里，不进入任何运行时依赖。

| 工具 | 作用 | 失败含义 |
|------|------|----------|
| `modgen` | 校验 29 个 `go.mod` 是否与 `modules.json` 一致 | 手改过 go.mod 或忘了同步 |
| `modrun` | 跨模块批量 `build` / `test` / `vet` / `tidy` / 依赖边界检查 | 某个模块编译不通过或越层引用 |
| `apicheck` | 从源码提取导出 API，生成 `api/api.txt` | 公开 API 快照未更新 |
| `doccheck` | 校验文档里的框架符号真实存在、示例语法正确 | 文档写了不存在的 API |
| `benchcheck` | 把 benchmark 结果与基线对比 | ns/op 或 allocs/op 回退 |

## 1. modules.json：模块清单的唯一事实来源

仓库是多模块工作区（`go.work`），但 `go.work` 只表达"哪些目录参与编译"，
不表达"哪个模块允许依赖谁"。`modules.json` 补上后者：

```json
{
  "modules": [
    { "path": "kernel/v3", "version": "v3.0.0", "deps": [] },
    { "path": "protocol/http/v3", "version": "v3.0.0", "deps": ["kernel/v3"] }
  ]
}
```

- `deps` 是**允许**的依赖（策略），不要求全部出现在 `require` 里；实际依赖仍由 `go mod tidy` 决定。
- 合入新模块后执行一次 `modgen -write`，它会补齐 `require`、`replace` 与 `go.work` 的 `use`。

```bash
cd tools
go run ./cmd/modgen -write -dir ..   # 按清单重写各 go.mod / go.work
go run ./cmd/modgen -check -dir ..   # CI 里只做校验
```

{{< callout type="info" >}}
`modgen -check` 会同时校验：`require` 是否为 `deps` 的子集、每个内部 `require` 是否有对应 `replace`、
`go.work` 是否覆盖全部模块、版本号是否为 `/v3` 模块的 `v3.0.0`。
{{< /callout >}}

## 2. modrun：跨模块执行

```bash
cd tools
go run ./cmd/modrun build      # 逐模块 go build ./...
go run ./cmd/modrun test -short
go run ./cmd/modrun vet
go run ./cmd/modrun tidy       # 逐模块 go mod tidy
go run ./cmd/modrun deps       # 依赖边界检查（用 go list -deps 找越层引用）
```

工作区模式下从仓库根执行 `go list ./...` 会失败（根模块不在 `go.work` 里），
`modrun` 因此按 `modules.json` 逐模块执行，并对 `tidy` 去掉 `./...` 参数。

`modrun deps` 只允许 `modules.json` 里声明的边，例如 `protocol/http/v3` 不允许 import
`integrations/orm/v3`。这条规则挡住的是"N 个模块悄悄长成一个巨型模块"。

## 3. apicheck：公开 API 快照

```bash
cd tools
go run ./cmd/apicheck -update -dir ..   # 重新生成 api/api.txt
go run ./cmd/apicheck -dir ..           # 只对比，有差异即失败
```

`api/api.txt` 是 52 个包的导出符号清单，配套测试 `test/api`：

```go
func TestExportedAPIStable(t *testing.T) { ... }   // -short 时跳过
```

改动公开 API 时，测试会打印 diff。确认是有意为之后再 `-update`，这个"多按一次键"的动作
就是 API 变更的显式确认，也便于在 PR 里看到究竟删了什么。

## 4. doccheck：文档与源码对齐

```bash
cd tools
go run ./cmd/doccheck -dir ..
```

它做两件事：

1. 所有 ```` ```go ```` 代码块里的 `alias.Symbol` 必须能在 `api/api.txt` 中找到对应导出符号；
2. 标记为 ```` ```go check ```` 的代码块额外做**语法**校验（文档里常见省略号写法，因此不是全部校验语法）。

```go check
type Guard interface {
	CanActivate(ctx *ExecutionContext) (bool, error)
}
```

配套测试 `test/docs` 让这条规则进 CI。历史上文档里出现过 `module.NewBaseModule`、
`aop.InvocationContext` 这类"看起来很像真的"的不存在符号，只有机器能稳定抓住。

## 5. benchcheck：性能基线

```bash
cd test
# 每项 benchmark 独立进程采集：同进程内后跑的 benchmark 会受前面留下的堆/GC 状态影响，
# 实测漂移可达 10%~20%，足以让 10% 阈值的门禁误报。
for b in BenchmarkHTTP_StaticJSON BenchmarkHTTP_AOPDepth10 BenchmarkHTTP_BindValidate BenchmarkDI_ResolveNested; do
  go test -run XXX -bench "^$b$" -benchmem -count 6 ./bench/ | grep '^Benchmark'
done > bench/current.txt

cd ../tools
go run ./cmd/benchcheck -baseline ../test/bench/baseline.txt -current ../test/bench/current.txt
go run ./cmd/benchcheck -write -baseline ../test/bench/baseline.txt -current ../test/bench/current.txt  # 接受新基线
```

默认阈值：ns/op 回退超过 10%、allocs/op 增加超过 1 即失败。
`allocs/op` 是确定值，比 ns/op 更稳，建议优先拿它当回归信号。
基线本身也会被 `test/bench` 里的测试校验可解析，避免格式漂移导致门禁形同虚设。

现有基准覆盖四条最容易劣化的路径：静态 JSON 响应、10 层中间件栈、绑定+校验、嵌套依赖注入。

### 热路径上刻意保留的约束

以下四点是「每请求零/低分配」的关键，改动时请连同 `benchcheck` 一起核对：

1. **静态路由走三层索引**（`Router.staticIndex`：protocol → method → path），
   不再每请求拼 `protocol:method:path` 字符串；新增/删除路由必须同时维护索引。
2. **`HandlerContext` 池化**：空 query 时完全跳过 `queryValues`，让池里的 map 继续复用。
3. **每请求只构造一个 `aop.ExecutionContext`**，middleware/guard/interceptor/filter 共享，
   因此各层看到的上下文对象一定相同。
4. **`di.Container` 的 providers/instances 懒分配**：请求级子容器通常全程只读，
   因此只付一次结构体分配，不付三张 map 的分配。


## 6. 推荐执行顺序

```bash
cd tools
go run ./cmd/modgen  -check -dir ..
go run ./cmd/modrun  build
go run ./cmd/modrun  deps
go run ./cmd/doccheck -dir ..

cd ../test
go vet ./...
go test ./...        # 含 api 门禁与 docs 门禁
```

整套跑完约 2 分钟（`test/api` 需要单独加载 52 个包，约占一半时间）。
