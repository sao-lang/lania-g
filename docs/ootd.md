# OOTD 微服务练手项目设计文档

## 1. 文档目标

这是一份**独立于当前仓库业务**的学习型项目设计文档，目标不是直接做一个生产级系统，而是设计一个**从网关开始、覆盖微服务常见分层和中间件**的 Demo，帮助你边搭边理解：

- 微服务为什么要分层。
- 常见中间件分别解决什么问题。
- 每一层的典型 API、核心原理、接入姿势和注意事项。
- 如何用 `docker compose` 一次性拉起依赖环境。
- 在真实数据量不大的情况下，如何通过 mock 数据和异步任务模拟“像样”的业务流量。

本文档默认你有前端和 Go 开发基础，但对微服务体系还是初学者。

## 2. 项目定位

项目名称建议：`ootd-lab`

业务主题选择：**OOTD 穿搭内容平台**

之所以选择这个题材，是因为它天然适合覆盖很多微服务场景：

- 用户体系：登录、鉴权、个人资料。
- 内容体系：发布穿搭、图文详情、标签、话题。
- 互动体系：点赞、收藏、评论、关注。
- 搜索体系：按关键词、品牌、风格搜索。
- Feed 体系：推荐流、热门流、关注流。
- 通知体系：评论通知、点赞通知、系统消息。
- 异步体系：内容审核、搜索索引更新、统计聚合。

这套业务不复杂，但足够把常见中间件串起来。

## 3. 学习目标

你做完这个项目后，应该能真正掌握下面这些点：

1. 明白网关层、应用层、服务层、数据层、中间件层分别负责什么。
2. 理解同步调用和异步消息的边界，知道什么时候该用 `HTTP`，什么时候该用 `gRPC`，什么时候该用 `Kafka`。
3. 明白 `Redis`、`MySQL`、`Elasticsearch` 各自适合承接什么类型的数据。
4. 知道配置中心、注册发现、链路追踪、指标监控为什么在微服务里几乎是标配。
5. 知道 Demo 项目和真实大规模生产系统的差异在哪里，哪些设计是“学习有效”，哪些是“生产必须谨慎”。

## 4. 总体架构

### 4.1 分层视图

```text
client(web/mobile/postman)
  -> Nginx Gateway
  -> gateway-service
      -> auth-service
      -> user-service
      -> content-service
      -> interaction-service
      -> feed-service
      -> search-service
      -> notification-service
      -> admin-service

infra:
  -> Nacos        配置中心 + 服务注册发现
  -> Redis        缓存 / 分布式锁 / 计数器
  -> Kafka        事件总线
  -> MySQL        事务型主存储
  -> GORM         Go ORM
  -> Elasticsearch 搜索与聚合
  -> MinIO        对象存储
  -> OTel Collector
  -> Prometheus
  -> Grafana
  -> Jaeger
```

### 4.2 推荐调用关系

```text
外部请求:
browser/app
  -> nginx
  -> gateway-service(http)
  -> 内部服务(http/grpc)

内部同步:
gateway-service -> auth-service
gateway-service -> content-service
feed-service -> user-service
feed-service -> interaction-service

内部异步:
content-service 发布内容
  -> kafka topic: content.published
  -> search-service 消费后写入 es
  -> notification-service 消费后发通知
  -> feed-service 消费后刷新推荐池

缓存旁路:
service -> redis -> mysql

搜索链路:
search-service -> elasticsearch
```

## 5. 服务拆分设计

### 5.1 服务列表

| 服务 | 职责 | 对外协议 | 数据依赖 |
| --- | --- | --- | --- |
| `gateway-service` | 鉴权透传、路由聚合、限流、统一错误码、灰度入口 | HTTP | Nacos、Redis |
| `auth-service` | 登录、JWT、刷新令牌、权限校验 | HTTP + gRPC | MySQL、Redis |
| `user-service` | 用户资料、关注关系、用户标签 | HTTP + gRPC | MySQL、Redis |
| `content-service` | 发布 OOTD、详情、标签、上下架、审核状态 | HTTP + gRPC | MySQL、Redis、MinIO、Kafka |
| `interaction-service` | 点赞、收藏、评论、计数聚合 | HTTP + gRPC | MySQL、Redis、Kafka |
| `feed-service` | 关注流、热门流、推荐流、分页游标 | HTTP + gRPC | Redis、Kafka、MySQL |
| `search-service` | 全文搜索、筛选、聚合 | HTTP | Elasticsearch、Kafka |
| `notification-service` | 站内信、评论通知、系统通知 | HTTP + Kafka Consumer | MySQL、Kafka、Redis |
| `admin-service` | 后台审核、内容巡检、运营位管理 | HTTP | MySQL、ES |
| `mock-data-service` | 生成假用户、假内容、假行为、定时压测事件 | Cron + Kafka Producer | MySQL、Kafka、Redis |

### 5.2 为什么这样拆

- `auth-service` 单独拆出，是为了把鉴权和业务解耦。
- `content-service` 与 `interaction-service` 分离，是为了区分“主数据写入”和“高频行为写入”。
- `feed-service` 单独存在，是因为推荐流和聚合流的性能策略通常独立于内容服务。
- `search-service` 单独存在，是为了让 `ES` 的索引更新和查询模型独立演进。
- `notification-service` 通过消息消费驱动，适合学习异步解耦。
- `mock-data-service` 是 Demo 的关键，它负责模拟真实流量和数据规模。

## 6. 推荐技术栈总表

### 6.1 必选技术栈

| 类别 | 技术选型 | 作用 |
| --- | --- | --- |
| 网关 | `Nginx` + `gateway-service` | 七层路由、静态入口、反向代理、统一入口 |
| 配置中心 / 注册发现 | `Nacos` | 集中配置、服务注册、动态下发 |
| 服务间同步通信 | `gRPC` | 高性能内部 RPC |
| 外部 API | `HTTP/JSON` | 给前端调试更直观 |
| 缓存 | `Redis` | 热点缓存、计数器、分布式锁、会话 |
| 消息队列 | `Kafka` | 解耦异步链路、削峰、事件驱动 |
| 主数据库 | `MySQL 8` | 事务型业务数据 |
| ORM | `GORM` | Go 项目快速接库 |
| 搜索 | `Elasticsearch` | 搜索、筛选、聚合 |
| 对象存储 | `MinIO` | 图片上传、封面地址管理 |
| 可观测性 | `OpenTelemetry` + `Jaeger` + `Prometheus` + `Grafana` | trace、metrics、监控面板 |
| 容器编排 | `Docker Compose` | 本地快速拉起依赖 |

### 6.2 建议补充工具

| 工具 | 用途 |
| --- | --- |
| `Kafka UI` | 浏览 topic、消息、consumer group |
| `phpMyAdmin` 或 `Adminer` | 直观查看 MySQL 数据 |
| `Kibana` | 查看 ES 索引和搜索效果 |
| `Redis Insight` | 查看 key、TTL、内存情况 |
| `protobuf` + `buf` | 管理 gRPC 协议文件 |
| `Makefile` | 简化本地命令 |

## 7. 目录结构建议

```text
ootd-lab/
  apps/
    gateway-service/
    auth-service/
    user-service/
    content-service/
    interaction-service/
    feed-service/
    search-service/
    notification-service/
    admin-service/
    mock-data-service/
  api/
    proto/
    openapi/
  deploy/
    docker-compose.yml
    nginx/
      nginx.conf
    nacos/
      application.properties
    prometheus/
      prometheus.yml
    otel/
      otel-collector-config.yaml
  pkg/
    xhttp/
    xgrpc/
    xkafka/
    xredis/
    xmysql/
    xtrace/
    xlog/
  scripts/
    seed/
    mock/
    benchmark/
  docs/
    architecture.md
    api.md
    runbook.md
```

## 8. 核心业务流程

### 8.1 用户发布 OOTD

```text
client
  -> gateway-service
  -> auth-service 校验 token
  -> content-service 写 mysql
  -> content-service 发 kafka 事件 content.published
  -> search-service 消费消息并写 es
  -> feed-service 消费消息并更新推荐池/关注流候选
  -> notification-service 消费消息并给关注者发提醒
```

### 8.2 用户点赞内容

```text
client
  -> gateway-service
  -> interaction-service
  -> redis 做去重或计数缓冲
  -> mysql 异步/准实时落库
  -> kafka 发出 interaction.liked
  -> notification-service 消费后发通知
  -> content-service 或 feed-service 更新聚合计数
```

### 8.3 用户搜索内容

```text
client
  -> gateway-service
  -> search-service
  -> elasticsearch
  -> 返回搜索结果
```

## 9. 中间件与组件分层说明

### 9.1 网关层

组件：

- `Nginx`
- `gateway-service`

职责：

- 统一入口。
- 路由分发。
- CORS、静态资源、压缩。
- TLS 终止。
- 基础限流。
- 请求头透传，例如 `X-Request-Id`、`Authorization`。
- 聚合多个内部服务结果，给前端一个更稳定的 API 面。

推荐实践：

- `Nginx` 负责通用网关职责，`gateway-service` 负责业务网关职责。
- 不要把过多业务逻辑塞到 `Nginx`。
- 不要让前端直接调用所有内部服务。

### 9.2 配置中心与注册发现层

组件：

- `Nacos`

职责：

- 服务启动时注册实例。
- 网关和服务发现可用实例。
- 管理环境配置，比如数据库地址、Redis 地址、Kafka topic、灰度开关。

推荐实践：

- 本地 Demo 允许固定地址 + Nacos 双模式。
- 先理解配置拉取和实例注册，再考虑动态变更和灰度。

### 9.3 通信层

组件：

- 对外：`HTTP`
- 对内：`gRPC`
- 异步：`Kafka`

职责：

- `HTTP` 适合前端直接调试，接口直观。
- `gRPC` 适合服务间同步调用，性能好，契约清晰。
- `Kafka` 适合异步解耦、削峰、事件广播。

### 9.4 存储层

组件：

- `MySQL`
- `Redis`
- `Elasticsearch`
- `MinIO`

职责划分：

- `MySQL`：订单式、关系型、事务型主数据。
- `Redis`：热点数据、临时态、计数器、排行榜、分布式锁。
- `Elasticsearch`：搜索索引、聚合分析、复杂检索。
- `MinIO`：图片和媒体文件存储。

### 9.5 可观测性层

组件：

- `OpenTelemetry`
- `Jaeger`
- `Prometheus`
- `Grafana`

职责：

- trace：一次请求在多个服务里的完整链路。
- metrics：QPS、延迟、错误率、缓存命中率。
- dashboard：把运行状态可视化。

## 10. docker compose 依赖设计

下面是一个**学习型环境**的 `docker-compose.yml` 示例，只拉基础中间件，不强制把所有业务服务都放进 compose；业务服务可以本地启动，方便调试。

```yaml
version: "3.9"

services:
  mysql:
    image: mysql:8.0
    container_name: ootd-mysql
    environment:
      MYSQL_ROOT_PASSWORD: root
      MYSQL_DATABASE: ootd
    ports:
      - "3306:3306"
    command:
      [
        "mysqld",
        "--default-authentication-plugin=mysql_native_password",
        "--character-set-server=utf8mb4",
        "--collation-server=utf8mb4_unicode_ci"
      ]
    volumes:
      - mysql_data:/var/lib/mysql

  redis:
    image: redis:7
    container_name: ootd-redis
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

  nacos:
    image: nacos/nacos-server:v2.3.2
    container_name: ootd-nacos
    environment:
      MODE: standalone
    ports:
      - "8848:8848"
    depends_on:
      - mysql

  zookeeper:
    image: confluentinc/cp-zookeeper:7.5.0
    container_name: ootd-zookeeper
    environment:
      ZOOKEEPER_CLIENT_PORT: 2181
    ports:
      - "2181:2181"

  kafka:
    image: confluentinc/cp-kafka:7.5.0
    container_name: ootd-kafka
    depends_on:
      - zookeeper
    ports:
      - "9092:9092"
    environment:
      KAFKA_BROKER_ID: 1
      KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1

  kafka-ui:
    image: provectuslabs/kafka-ui:latest
    container_name: ootd-kafka-ui
    depends_on:
      - kafka
    ports:
      - "8081:8080"
    environment:
      KAFKA_CLUSTERS_0_NAME: local
      KAFKA_CLUSTERS_0_BOOTSTRAPSERVERS: kafka:9092

  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.13.4
    container_name: ootd-es
    environment:
      discovery.type: single-node
      xpack.security.enabled: "false"
      ES_JAVA_OPTS: "-Xms1g -Xmx1g"
    ports:
      - "9200:9200"
    volumes:
      - es_data:/usr/share/elasticsearch/data

  kibana:
    image: docker.elastic.co/kibana/kibana:8.13.4
    container_name: ootd-kibana
    depends_on:
      - elasticsearch
    ports:
      - "5601:5601"
    environment:
      ELASTICSEARCH_HOSTS: http://elasticsearch:9200

  minio:
    image: minio/minio:latest
    container_name: ootd-minio
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: minio
      MINIO_ROOT_PASSWORD: minio123
    ports:
      - "9000:9000"
      - "9001:9001"
    volumes:
      - minio_data:/data

  jaeger:
    image: jaegertracing/all-in-one:1.57
    container_name: ootd-jaeger
    ports:
      - "16686:16686"
      - "4317:4317"
      - "4318:4318"

  prometheus:
    image: prom/prometheus:latest
    container_name: ootd-prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml

  grafana:
    image: grafana/grafana:latest
    container_name: ootd-grafana
    ports:
      - "3001:3000"

volumes:
  mysql_data:
  redis_data:
  es_data:
  minio_data:
```

### 10.1 Demo 环境建议

- 所有中间件单节点即可。
- 不需要一上来就做高可用。
- 不建议一开始引入 `Kubernetes`，先在 `docker compose` 把链路跑通。

## 11. Nginx 配置示例

这是最基础的网关入口配置，重点是理解反向代理、请求头透传和静态入口。

```nginx
worker_processes  1;

events {
    worker_connections  1024;
}

http {
    include       mime.types;
    default_type  application/octet-stream;
    sendfile      on;
    keepalive_timeout 65;

    upstream gateway_backend {
        server host.docker.internal:8080;
    }

    server {
        listen 80;
        server_name localhost;

        location /api/ {
            proxy_pass http://gateway_backend/;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Request-Id $request_id;
            proxy_set_header Authorization $http_authorization;
        }

        location / {
            root /usr/share/nginx/html;
            index index.html;
            try_files $uri /index.html;
        }
    }
}
```

### 11.1 Nginx 核心原理

- `upstream` 表示后端服务池。
- `proxy_pass` 表示把请求转发给后端。
- `location` 是路由匹配规则。
- `proxy_set_header` 用于透传真实请求上下文。

### 11.2 Nginx 使用场景

- 单入口反向代理。
- 前后端同域部署。
- 静态资源托管。
- 简单限流和连接控制。

### 11.3 Nginx 注意点

- 业务鉴权逻辑不要全部堆进 `Nginx`。
- 上传文件时注意 body 限制和超时配置。
- 生产中通常会加健康检查、TLS、日志切割、限流模块。

## 12. 配置中心与服务发现

这里推荐使用 `Nacos`，因为它在学习项目里比较省心：**配置中心和注册发现二合一**。

### 12.1 使用场景

- 多服务共享配置。
- 根据环境切换数据库和缓存地址。
- 服务实例动态注册，网关能发现可用节点。

### 12.2 示例配置结构

建议按服务拆分配置：

```yaml
server:
  port: 8082

mysql:
  dsn: root:root@tcp(localhost:3306)/ootd?charset=utf8mb4&parseTime=True&loc=Local // ignore_security_alert

redis:
  addr: localhost:6379
  db: 0

kafka:
  brokers:
    - localhost:9092
  topics:
    content_published: content.published
    interaction_liked: interaction.liked

es:
  addr: http://localhost:9200
  index: ootd_content
```

### 12.3 核心原理

- 配置中心本质上是“集中式配置存储 + 客户端拉取/监听变更”。
- 注册发现本质上是“服务启动时注册自己的地址，调用方按服务名找到实例地址”。

### 12.4 注意点

- 配置变更不等于服务一定安全热更新，很多配置仍建议重启生效。
- 服务发现解决的是“找到服务”，不是“服务一定可用”。
- Demo 中可以先只用注册功能，再逐步加上配置监听。

## 13. gRPC 设计

### 13.1 为什么要用 gRPC

- Go 服务间调用性能更高。
- `proto` 契约清晰，适合多人协作。
- 支持生成客户端和服务端代码。

### 13.2 推荐使用场景

- `gateway-service -> auth-service`
- `gateway-service -> user-service`
- `feed-service -> user-service`
- `feed-service -> interaction-service`

### 13.3 proto 示例

```proto
syntax = "proto3";

package ootd.content.v1;

option go_package = "ootd-lab/api/proto/content/v1;contentv1";

service ContentService {
  rpc GetContent(GetContentRequest) returns (GetContentResponse);
  rpc CreateContent(CreateContentRequest) returns (CreateContentResponse);
}

message GetContentRequest {
  int64 id = 1;
}

message GetContentResponse {
  int64 id = 1;
  int64 user_id = 2;
  string title = 3;
  string description = 4;
  repeated string tags = 5;
}

message CreateContentRequest {
  int64 user_id = 1;
  string title = 2;
  string description = 3;
  repeated string tags = 4;
}

message CreateContentResponse {
  int64 id = 1;
}
```

### 13.4 核心原理

- `proto` 是接口描述文件。
- 服务端实现接口，客户端基于 stub 发起调用。
- 数据默认走 protobuf 序列化，体积更小、速度更快。

### 13.5 注意点

- 对外开放接口仍建议保留 HTTP，前端调试体验更好。
- gRPC 更适合服务间通信，不要为了“技术酷”把所有入口都做成 gRPC。
- 需要考虑超时、重试、熔断、幂等。

## 14. MySQL 与 GORM

### 14.1 使用场景

- 用户表、内容表、评论表、关注关系表、通知表。
- 强一致写入、事务、关联查询。

### 14.2 建表示例

```sql
CREATE TABLE users (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  nickname VARCHAR(64) NOT NULL,
  avatar_url VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE contents (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  title VARCHAR(128) NOT NULL,
  description TEXT NOT NULL,
  cover_url VARCHAR(255) NOT NULL DEFAULT '',
  status TINYINT NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_user_id (user_id),
  KEY idx_created_at (created_at)
);
```

### 14.3 GORM 模型示例

```go
package model

import "time"

type Content struct {
	ID          int64     `gorm:"primaryKey"`
	UserID      int64     `gorm:"index;not null"`
	Title       string    `gorm:"size:128;not null"`
	Description string    `gorm:"type:text;not null"`
	CoverURL    string    `gorm:"size:255;not null;default:''"`
	Status      int32     `gorm:"not null;default:1"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
```

### 14.4 GORM 查询示例

```go
func (r *ContentRepo) GetByID(ctx context.Context, id int64) (*model.Content, error) {
	var content model.Content
	err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", id, 1).
		First(&content).Error
	if err != nil {
		return nil, err
	}
	return &content, nil
}
```

### 14.5 核心原理

- `MySQL` 负责最终持久化。
- `GORM` 是 Go 到 SQL 的映射层，本质不是替代 SQL，而是帮你管理模型和常见查询。

### 14.6 注意点

- 复杂查询不要完全迷信 ORM，必要时写原生 SQL。
- 建索引时围绕真实查询场景，不是字段越多越好。
- 不要在高频接口里滥用 `Preload`。

## 15. Redis 设计

### 15.1 使用场景

- 内容详情缓存。
- 热门榜单缓存。
- 点赞/收藏计数器。
- 用户登录态、refresh token。
- 分布式锁。
- Feed 游标和临时推荐池。

### 15.2 Go 代码示例

```go
func (s *ContentService) GetContentDetail(ctx context.Context, id int64) (*ContentDetail, error) {
	cacheKey := fmt.Sprintf("content:detail:%d", id)

	raw, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var detail ContentDetail
		if json.Unmarshal([]byte(raw), &detail) == nil {
			return &detail, nil
		}
	}

	detail, err := s.repo.GetContentDetail(ctx, id)
	if err != nil {
		return nil, err
	}

	buf, _ := json.Marshal(detail)
	_ = s.redis.Set(ctx, cacheKey, buf, 5*time.Minute).Err()

	return detail, nil
}
```

### 15.3 Redis 核心 API

- `GET` / `SET`
- `DEL`
- `EXPIRE`
- `INCR`
- `HGET` / `HSET`
- `ZADD` / `ZREVRANGE`
- `SETNX`

### 15.4 核心原理

- Redis 是内存型 key-value 存储，读写快。
- 常见数据结构包括 string、hash、list、set、zset。
- 适合承接高频访问和临时态数据，不适合作为唯一真实数据源。

### 15.5 注意点

- 缓存不是数据库，重要数据最终还是要落 MySQL。
- 需要考虑缓存穿透、缓存击穿、缓存雪崩。
- 分布式锁只是工具，不是万能并发解决方案。

## 16. Kafka 设计

### 16.1 使用场景

- 发布内容后，异步更新搜索索引。
- 点赞后，异步发通知。
- 评论后，异步聚合统计。
- mock-data-service 持续制造事件流。

### 16.2 Topic 规划

| Topic | 说明 |
| --- | --- |
| `content.published` | 内容发布事件 |
| `content.updated` | 内容更新事件 |
| `interaction.liked` | 点赞事件 |
| `interaction.commented` | 评论事件 |
| `notification.created` | 通知创建事件 |
| `mock.behavior.generated` | mock 行为事件 |

### 16.3 Producer 示例

```go
type ContentPublishedEvent struct {
	ContentID int64    `json:"content_id"`
	UserID    int64    `json:"user_id"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags"`
}

func (p *Producer) PublishContentCreated(ctx context.Context, evt ContentPublishedEvent) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return err
	}

	msg := &kafka.Message{
		Topic: "content.published",
		Key:   []byte(strconv.FormatInt(evt.ContentID, 10)),
		Value: body,
	}

	return p.writer.WriteMessages(ctx, *msg)
}
```

### 16.4 Consumer 示例

```go
func (c *Consumer) Run(ctx context.Context) error {
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			return err
		}

		var evt ContentPublishedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			continue
		}

		if err := c.searchIndexer.IndexContent(ctx, evt); err != nil {
			// 实际项目应记录日志并根据策略重试或进入死信队列
			continue
		}
	}
}
```

### 16.5 核心原理

- Producer 把消息发到 topic。
- topic 内部分为多个 partition。
- consumer group 共同消费分区，实现扩展和负载均衡。

### 16.6 注意点

- Kafka 不保证你的业务“刚好一次”，业务侧要自己考虑幂等。
- 分区键决定消息局部有序性。
- 消费失败后的重试、死信、补偿一定要设计。

## 17. Elasticsearch 设计

### 17.1 使用场景

- 关键词搜索。
- 标签筛选。
- 热门关键词统计。
- 多条件聚合查询。

### 17.2 索引示例

```json
PUT /ootd_content
{
  "mappings": {
    "properties": {
      "id": { "type": "long" },
      "user_id": { "type": "long" },
      "title": {
        "type": "text",
        "fields": {
          "keyword": { "type": "keyword" }
        }
      },
      "description": { "type": "text" },
      "tags": { "type": "keyword" },
      "status": { "type": "integer" },
      "created_at": { "type": "date" }
    }
  }
}
```

### 17.3 查询示例

```json
POST /ootd_content/_search
{
  "query": {
    "bool": {
      "must": [
        { "match": { "title": "夏日 通勤" } }
      ],
      "filter": [
        { "term": { "status": 1 } },
        { "terms": { "tags": ["简约", "通勤"] } }
      ]
    }
  },
  "sort": [
    { "created_at": "desc" }
  ],
  "from": 0,
  "size": 10
}
```

### 17.4 核心原理

- ES 不是关系型数据库，它是倒排索引搜索引擎。
- 适合“查得复杂、过滤多、搜索多”的场景。
- 通常通过异步消息从 MySQL 同步构建索引。

### 17.5 注意点

- 不要把 ES 当主数据库。
- ES 和 MySQL 之间常常是“最终一致”。
- 索引设计要围绕搜索字段和筛选字段。

## 18. MinIO 设计

### 18.1 使用场景

- OOTD 封面图。
- 内容详情图。
- 用户头像。

### 18.2 核心原理

- 对象存储按 bucket + object key 管理文件。
- 服务中通常只存文件 URL 或对象 key，不把大文件直接塞数据库。

### 18.3 注意点

- 上传成功但数据库写入失败时，要考虑补偿。
- 访问 URL 是否公开，需要根据业务控制。

## 19. OpenTelemetry、Jaeger、Prometheus、Grafana

### 19.1 使用场景

- 想知道一个请求慢在哪里，用 `trace`。
- 想知道服务最近 QPS、错误率、P99，用 `metrics`。
- 想看可视化大盘，用 `Grafana`。

### 19.2 推荐指标

- HTTP 请求总量
- HTTP 请求耗时
- gRPC 调用耗时
- Redis 命中率
- Kafka 消费滞后
- MySQL 慢查询数
- ES 查询耗时

### 19.3 核心原理

- `OpenTelemetry` 负责统一采集埋点。
- `Jaeger` 用于查看调用链。
- `Prometheus` 拉取并存储 metrics。
- `Grafana` 用于可视化。

### 19.4 注意点

- 监控不是越多越好，先监控关键路径。
- trace 采样率要控制，不然开销会很大。

## 20. API 设计建议

### 20.1 对外 HTTP API 示例

```http
POST /api/v1/auth/login
GET  /api/v1/users/me
POST /api/v1/contents
GET  /api/v1/contents/{id}
POST /api/v1/contents/{id}/like
GET  /api/v1/feed/recommend
GET  /api/v1/search?q=通勤穿搭&tag=简约
GET  /api/v1/notifications
```

### 20.2 返回结构建议

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

### 20.3 错误码建议

| code | 含义 |
| --- | --- |
| `0` | 成功 |
| `40001` | 参数错误 |
| `40101` | 未登录 |
| `40301` | 无权限 |
| `40401` | 资源不存在 |
| `40901` | 资源冲突 |
| `50001` | 系统内部错误 |

## 21. mock 数据方案

这是 Demo 项目最重要的一部分，因为没有真实用户量时，中间件能力很难体现出来。

### 21.1 目标

用少量机器、少量真实用户，模拟出：

- 有用户注册。
- 有内容持续产生。
- 有点赞、评论、收藏、关注行为。
- 有消息流转。
- 有缓存命中。
- 有搜索索引更新。
- 有监控图表波动。

### 21.2 mock-data-service 设计

建议做一个独立服务，支持下面几类任务：

1. 造用户
2. 造内容
3. 造互动行为
4. 定时发送 Kafka 事件
5. 模拟热门内容被集中访问
6. 模拟白天高峰、夜间低峰的访问曲线

### 21.3 mock 方式

#### 方式一：初始化种子数据

启动脚本执行：

- 生成 1000 个用户
- 每个用户生成 10 到 30 条内容
- 每条内容随机分配风格标签、品牌标签、季节标签

#### 方式二：定时行为生成

每分钟执行一次任务：

- 随机用户点赞 500 次
- 随机用户评论 100 次
- 随机用户收藏 200 次
- 随机用户搜索 300 次

#### 方式三：热点流量模拟

挑选 20 条“爆款内容”，对它们重复发起：

- 详情查询
- 点赞
- 搜索命中

这样可以明显观察到：

- Redis 热 key
- Kafka 消费堆积
- ES 查询压力
- Prometheus 指标波动

### 21.4 mock 数据生成示例

```go
type MockContent struct {
	UserID      int64
	Title       string
	Description string
	Tags        []string
}

func randomContent(userID int64) MockContent {
	titles := []string{
		"今日通勤穿搭",
		"夏日极简风",
		"周末咖啡店 OOTD",
		"雨天叠穿灵感",
	}
	tagsPool := [][]string{
		{"通勤", "简约", "夏日"},
		{"复古", "叠穿", "街头"},
		{"法式", "连衣裙", "约会"},
	}

	return MockContent{
		UserID:      userID,
		Title:       titles[rand.Intn(len(titles))],
		Description: "这是一条用于学习微服务架构的 mock 内容",
		Tags:        tagsPool[rand.Intn(len(tagsPool))],
	}
}
```

### 21.5 mock 的核心原则

- 不追求绝对真实，追求“足以触发系统行为”。
- 要让缓存、搜索、消息队列、监控都能看到效果。
- 要有热点分布，而不是完全均匀的随机数据。

## 22. 关键工程实践

### 22.1 统一请求链路

每个请求都带上：

- `request_id`
- `user_id`
- `trace_id`

这样查日志和查 trace 时会轻松很多。

### 22.2 幂等设计

重点接口要考虑幂等：

- 点赞
- 收藏
- 发布内容重试
- Kafka 消费重复处理

### 22.3 超时和重试

- gRPC 调用必须带超时。
- 重试只用于可安全重试的操作。
- 写操作要谨慎重试，避免重复数据。

### 22.4 缓存策略

建议优先练这三种：

- Cache Aside
- Read Through 思想
- 延迟双删思路

### 22.5 事件驱动边界

适合异步事件的场景：

- 发通知
- 更新搜索索引
- 聚合统计
- 推荐池刷新

不适合异步的场景：

- 登录立即返回 token
- 下单立即扣库存这一类强一致操作

## 23. 分阶段落地路线

### 第 1 阶段：最小可运行

目标：

- `Nginx + gateway-service + auth-service + user-service + content-service`
- `MySQL + Redis`
- 跑通登录、发内容、查详情

### 第 2 阶段：服务注册与 gRPC

目标：

- 接入 `Nacos`
- 服务间改为 `gRPC`
- 网关通过服务发现调用内部服务

### 第 3 阶段：消息驱动

目标：

- 接入 `Kafka`
- 发布内容后异步更新搜索和通知
- 增加 `notification-service`

### 第 4 阶段：搜索与 Feed

目标：

- 接入 `Elasticsearch`
- 增加 `search-service`
- 增加 `feed-service`

### 第 5 阶段：可观测性

目标：

- 接入 `OpenTelemetry`
- 接入 `Jaeger`、`Prometheus`、`Grafana`
- 做一套基础大盘

### 第 6 阶段：mock 数据和压测

目标：

- 增加 `mock-data-service`
- 持续造数据
- 观察缓存命中、消费延迟、搜索耗时

## 24. 每个技术栈最该掌握的点

| 技术 | 最该掌握的点 |
| --- | --- |
| `Nginx` | 反向代理、转发头、静态入口、基础限流 |
| `Nacos` | 配置拉取、服务注册、服务发现 |
| `gRPC` | proto、stub、超时、拦截器 |
| `Redis` | 数据结构、过期、热点 key、缓存策略 |
| `Kafka` | topic、partition、consumer group、幂等 |
| `MySQL` | 索引、事务、慢查询、分页 |
| `GORM` | 模型映射、链式查询、事务、原生 SQL |
| `Elasticsearch` | mapping、倒排索引、match/filter、聚合 |
| `MinIO` | bucket、对象 key、上传流程 |
| `OpenTelemetry` | trace/span、上下文透传 |
| `Prometheus` | 指标暴露、label、抓取模型 |
| `Grafana` | dashboard、面板组合 |

## 25. 这个 Demo 的价值边界

你要明确，这个项目是为了**建立微服务心智模型**，不是为了假装做了一个真实大厂系统。

它能帮你学会：

- 中间件为什么存在。
- 微服务链路怎么串。
- 配置、缓存、消息、搜索、监控怎么落地。

但它不一定天然覆盖：

- 真正复杂的分布式事务。
- 多机房容灾。
- 大规模流量治理。
- 多集群多租户隔离。

也正因为如此，学习路径应该是：

- 先跑通
- 再理解
- 再压测
- 再优化

而不是一开始就把系统设计得特别重。

## 26. 最终推荐结论

如果你想做一个**学习价值最大**、又不会一上来复杂到失控的微服务 Demo，我推荐最终组合如下：

- 网关：`Nginx + gateway-service`
- 配置与发现：`Nacos`
- 同步通信：`HTTP + gRPC`
- 缓存：`Redis`
- 消息：`Kafka`
- 主库：`MySQL`
- ORM：`GORM`
- 搜索：`Elasticsearch`
- 文件：`MinIO`
- 可观测性：`OpenTelemetry + Jaeger + Prometheus + Grafana`
- 容器：`Docker Compose`
- 数据模拟：`mock-data-service + seed 脚本 + 定时行为任务`

这个组合已经足够把微服务中的常见层基本覆盖完整，并且每个组件都能在这个 OOTD 题材里找到自然的落点。

如果后续你要继续扩展，可以再增加：

- `RabbitMQ`，对比和 `Kafka` 的差异
- `Sentinel` 或类似治理组件，学习限流熔断
- `CI/CD` 流程，学习镜像构建和发布
- `Kubernetes`，学习容器编排和服务治理
