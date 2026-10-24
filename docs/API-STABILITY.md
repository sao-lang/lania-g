# 公开 API 稳定性承诺

本文定义 `lania-g` 对外暴露的 API 范围，以及哪些改动属于破坏性变更。

## 1. 分层承诺

| 层级 | 位置 | 承诺等级 | 允许的变更 |
|------|------|----------|-----------|
| **Stable** | `kernel/v3`、`application/v3`、`protocol/*/v3` 的导出符号 | 稳定 | 只能新增；删除/改名/改签名需升 major |
| **Experimental** | `integrations/*/v3` 的导出符号 | 实验 | 允许 minor 版本调整，需在 CHANGELOG 说明 |
| **Internal** | 任意 `internal/` 子包、未导出符号 | 无承诺 | 随时变更 |
| **CLI / Demo** | `cmd/*` | 无承诺 | 仅为示例，随时重写 |

判断标准：**用户 import 了它，就要给承诺**。`cmd/` 下的包是 `package main`，无法被 import，因此不在承诺范围内。

## 2. 什么算破坏性变更

| 变更 | 破坏性 |
|------|--------|
| 删除或重命名导出符号 | ✅ |
| 修改函数签名（含参数顺序、类型） | ✅ |
| 修改导出 struct 的字段类型 | ✅ |
| 给导出 struct 新增字段 | ❌（前提：零值可用） |
| 给导出 interface 新增方法 | ✅ |
| 修改函数返回值语义（不报错的错误、不同的默认值） | ✅ |
| 修改错误消息文案 | ❌（不要依赖文案，用 `errors.Is`/`Kind`） |
| 新增导出符号 | ❌ |
| 修改 `internal/` 或未导出符号 | ❌ |

## 3. 自动化保障

仓库内置 API 契约快照（`api/*.txt`），由 `tools/apicheck` 生成：

```bash
go run ./tools/apicheck          # 校验当前代码与快照一致
go run ./tools/apicheck -update  # 预期变更后更新快照（需评审 diff）
```

CI 会运行 `go test ./test/api`，**任何导出符号的增删改都会失败**。
这保证"删除/改名不可能悄悄发生"——必须显式改快照，并在 PR 里被 review 到。

## 4. 关于 `v3`

`kernel/v3`、`protocol/http/v3` 等目录名里的 `v3` 是 **Go module major version**
的标准写法（module path 必须以 `/vN` 结尾才能发布 v2+ 版本），不是历史遗留。
它在源码目录里出现，是为了让 module 路径与磁盘路径一致（Go 的硬性要求）。

`v3` 是本项目的首个 major 版本号，不代表此前存在 v1/v2 用户。

## 5. 发布：每个 module 一个 tag

本仓库是多 module 仓库，`go get` 依赖 **git tag**，不需要额外发包。
每个子 module 必须打一个**带 module 目录前缀**的 annotated tag：

```text
<module 目录>/<版本>        例如  kernel/v3.0.0、protocol/http/v3.0.0
```

⚠️ 两个高频错误：

| 错误写法 | 为什么错 |
|----------|----------|
| `kernel/v3/v3.0.0` | 把 `module path` 的 `/v3` 后缀误当成目录层级，多了一层 |
| 轻量 tag（`git tag x`） | Go 解析依赖 tag 对象，部分场景解析不到，必须用 `git tag -a` |

正确命名由 module 目录**去掉末尾 `/v3`** 后接 `/v3.0.0` 得到：

```bash
# 为全部 25 个 module 打 tag
for d in kernel/v3 application/v3 protocol/*/v3 integrations/*/v3; do
  git tag -a "${d%/v3}/v3.0.0" -m "lania-g $d v3.0.0"
done
git push github --tags
```

发布后仓库内各 `go.mod` 中的内部依赖统一写 `v3.0.0`（`tools/modgen` 保证这一点），
外部项目因此可以直接 `go get github.com/sao-lang/lania-g/application/v3@v3.0.0`。

## 6. 弃用流程

本项目采用**直接删除**策略，不保留 deprecated 别名：

1. 在 PR 中删除旧符号，同时更新 `api/*.txt`
2. 在 CHANGELOG 标注 `BREAKING`
3. 给出替代写法

理由：保留别名会让 API 表面积无限增长，且两套入口本身就是缺陷源。
在没有广泛外部用户的阶段，直接删除的成本远低于长期维护成本。
