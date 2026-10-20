# docs/

这个目录只保留**面向使用者的活文档**。完整文档站在 [`website/`](../website)。

```text
docs/
├── README.md              # 本文件：文档地图
├── API-STABILITY.md       # 公开 API 的稳定性承诺
├── LAYERING.md            # 分层规则：什么逻辑放 AOP、什么放 Transport
├── 标准接入模板.md         # 从零接入一个服务的推荐写法
├── API.md                 # 历史 API 全览（正在被文档站取代）
└── archive/               # 历史设计文档，只读，不再维护
```

## 我要找什么？

| 需求 | 去哪 |
|------|------|
| 快速跑起来 | [website/getting-started](../website/content/docs/getting-started) |
| 查某个 API 签名 | [website/reference](../website/content/docs/reference) |
| 理解编译 / 运行期原理 | [website/internals](../website/content/docs/internals) |
| 接入某个集成（auth/orm/otel…） | [website/integrations](../website/content/docs/integrations) |
| 什么是稳定 API | [API-STABILITY.md](./API-STABILITY.md) |
| 该把逻辑放在哪一层 | [LAYERING.md](./LAYERING.md) |
| 历史设计讨论 | [archive/](./archive)（只读） |

## 关于 archive/

`archive/` 里的文档写于不同阶段，**可能与当前代码不一致**，保留价值是记录设计推理过程。
需要准确行为时，一律以文档站与源码为准。

## 关于 `v3`

目录名和 module 路径里的 `v3`（`github.com/sao-lang/lania-g/kernel/v3`）是 Go
major version 的标准写法，不是历史遗留，也不能省略。详见 [API-STABILITY.md](./API-STABILITY.md)。
