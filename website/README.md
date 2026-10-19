# lania-g 文档站

基于 **Hugo + [Hextra](https://github.com/imfing/hextra)**，无需 Node.js。

## 本地预览

```bash
# 首次需要拉取主题模块（使用 Go proxy）
hugo mod tidy --source website

# 启动预览（默认 http://localhost:1313）
hugo server --source website
```

## 构建

```bash
hugo --source website --minify
# 产物在 website/public/

# 严格构建（告警即失败，CI 用）
hugo --source website --minify --cleanDestinationDir --panicOnWarning --logLevel warn
```

## 目录

```text
website/
├── hugo.yaml              # 站点配置（导航、搜索、主题参数）
├── go.mod / go.sum        # Hugo Modules（主题依赖）
├── assets/
│   ├── css/custom.css     # 视觉主题（品牌色、排版、首页组件、暗色、响应式）
│   └── js/                # vendored FlexSearch / Mermaid（构建期不访问 CDN）
├── layouts/
│   └── _markup/render-link.html   # 相对链接解析钩子（见下方「写作约定」）
└── content/
    ├── _index.md          # 首页（自定义 hero / 卡片）
    └── docs/
        ├── getting-started/   入门
        ├── core-concepts/     核心概念
        ├── features.md        特性总览
        ├── syntax.md          语法速查
        ├── protocols/         协议指南
        ├── guides/            完整流程
        ├── reference/         参考手册（全量 API）
        ├── integrations/      集成
        ├── internals/         深入原理
        └── best-practices/    最佳实践
```

## 写作约定

1. 每页 front matter 至少包含 `title` 与 `weight`
2. 页首用一句话说明"这篇解决什么问题"
3. 示例代码必须来自已通过端到端验证的 `cmd/` 示例
4. 原理类文章给出 Mermaid 时序/数据流图
5. 页尾给出"下一步"链接，形成阅读路径
6. **相对链接按「源文件目录」书写**（如 `docs/features.md` 里写 `core-concepts/di`），
   `layouts/_markup/render-link.html` 会用 `site.GetPage` 解析成正确的 `RelPermalink`
7. `prev` / `next` 必须写站内绝对路径（如 `/docs/reference/http`），
   否则 `protocols/http` 与 `reference/http` 同名时会触发 `page reference ... is ambiguous`

## 主题说明

- 主题通过 Hugo Modules 引入，不复制到仓库
- `hugo.yaml` 中 `module.imports` 指向 `github.com/imfing/hextra`
- 搜索使用 FlexSearch（构建时索引 `content`）
- 自定义样式放在 `assets/css/custom.css`，Hextra 会自动加载
- `params.highlight.copy` 必须是 `copy.enable: true` 结构，写成 `copy: true` 会导致全站渲染失败
