---
title: 安装
weight: 1
prev: /docs/getting-started
next: /docs/getting-started/quick-start
---

本章说明如何准备 `lania-g` 的开发环境。

## 前置条件

| 依赖 | 版本 | 说明 |
|------|------|------|
| Go | 1.25+ | 仓库使用 `go.work` 多模块工作区 |
| Git | 任意 | 拉取仓库 |

```bash
go version
# go version go1.25.x windows/amd64
```

## 获取代码

```bash
git clone https://github.com/sao-lang/lania-g.git
cd lania-g
```

仓库根目录的 `go.work` 已经列出了全部子模块：

```text
application/v3
kernel/v3
protocol/{http,grpc,graphql,ws,mq,scheduler}/v3
integrations/*/v3
cmd
```

## 验证环境

每个子模块都是独立的 Go module，因此需要逐个目录执行：

```bash
# Windows PowerShell
$mods = Get-ChildItem -Path application,cmd,integrations,kernel,protocol -Recurse -Filter go.mod |
  ForEach-Object { $_.Directory.FullName }
foreach ($m in $mods) { Push-Location $m; go build ./...; go test ./...; Pop-Location }
```

```bash
# macOS / Linux
for d in $(find application cmd integrations kernel protocol -name go.mod -exec dirname {} \;); do
  (cd "$d" && go build ./... && go test ./...)
done
```

全部通过后再开始写业务代码。

## 运行示例

示例都支持环境变量覆盖端口，避免与本机已占用端口冲突：

```bash
cd cmd

# HTTP 示例（默认 :8080）
PORT=18080 go run ./http-app-demo
# 浏览器访问 http://127.0.0.1:18080/users/ping

# gRPC 示例（默认 :50051）
GRPC_ADDR=:19051 go run ./grpc-app-demo
```

{{< callout type="info" >}}
Windows PowerShell 的设置方式：`$env:PORT='18080'; go run ./http-app-demo`
{{< /callout >}}

## 常见问题

- **`go build ./...` 报 `directory prefix . does not contain modules`**
  - 说明你在仓库根目录执行。根目录没有 `go.mod`，请进入具体子模块或在 `cmd` 下执行。
- **端口被占用**
  - 使用 `PORT` / `APP_ADDR` / `GRPC_ADDR` 覆盖。
