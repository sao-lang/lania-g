// indicators.go 定义 terminus 集成的健康指标接口与常用指标实现。
package terminus

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"
)

// RedisPinger 定义 Redis 健康检查所需的最小探测能力。
type RedisPinger interface {
	Ping() error
}

// DatabaseIndicator 用于探测数据库连通性。
type DatabaseIndicator struct {
	name        string
	db          *sql.DB
	componentID string
}

// NewDatabaseIndicator 创建一个数据库健康检查指标。
func NewDatabaseIndicator(name string, db *sql.DB, componentID ...string) *DatabaseIndicator {
	id := ""
	if len(componentID) > 0 {
		id = componentID[0]
	}
	return &DatabaseIndicator{name: name, db: db, componentID: id}
}

// Name 返回指标名称。
func (d *DatabaseIndicator) Name() string { return d.name }

// Check 执行数据库连通性检查，并产出健康检查结果。
func (d *DatabaseIndicator) Check() (*CheckResult, error) {
	result := &CheckResult{
		Status:        StatusPass,
		ComponentType: "datastore",
		ComponentID:   d.componentID,
		Time:          time.Now(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.db.PingContext(ctx); err != nil {
		result.Status = StatusFail
		result.Output = err.Error()
		return result, nil
	}
	return result, nil
}

// RedisIndicator 用于探测 Redis 连通性。
type RedisIndicator struct {
	name        string
	pinger      func() error
	componentID string
}

// NewRedisIndicator 基于 RedisPinger 创建一个 Redis 健康检查指标。
func NewRedisIndicator(name string, pinger RedisPinger, componentID ...string) *RedisIndicator {
	id := ""
	if len(componentID) > 0 {
		id = componentID[0]
	}
	return &RedisIndicator{name: name, pinger: pinger.Ping, componentID: id}
}

// NewRedisIndicatorFunc 基于普通函数创建一个 Redis 健康检查指标。
func NewRedisIndicatorFunc(name string, pinger func() error, componentID ...string) *RedisIndicator {
	id := ""
	if len(componentID) > 0 {
		id = componentID[0]
	}
	return &RedisIndicator{name: name, pinger: pinger, componentID: id}
}

// Name 返回指标名称。
func (r *RedisIndicator) Name() string { return r.name }

// Check 执行 Redis 连通性检查，并产出健康检查结果。
func (r *RedisIndicator) Check() (*CheckResult, error) {
	result := &CheckResult{
		Status:        StatusPass,
		ComponentType: "cache",
		ComponentID:   r.componentID,
		Time:          time.Now(),
	}
	if err := r.pinger(); err != nil {
		result.Status = StatusFail
		result.Output = err.Error()
	}
	return result, nil
}

// HTTPIndicator 用于探测某个 HTTP 端点的可用性。
type HTTPIndicator struct {
	name        string
	url         string
	client      *http.Client
	componentID string
}

// NewHTTPIndicator 创建一个 HTTP 健康检查指标。
func NewHTTPIndicator(name, url string, componentID ...string) *HTTPIndicator {
	id := ""
	if len(componentID) > 0 {
		id = componentID[0]
	}
	return &HTTPIndicator{
		name:        name,
		url:         url,
		client:      &http.Client{Timeout: 5 * time.Second},
		componentID: id,
	}
}

// Name 返回指标名称。
func (h *HTTPIndicator) Name() string { return h.name }

// Check 执行 HTTP 端点探测，并产出健康检查结果。
func (h *HTTPIndicator) Check() (*CheckResult, error) {
	result := &CheckResult{
		Status:        StatusPass,
		ComponentType: "system",
		ComponentID:   h.componentID,
		Time:          time.Now(),
	}
	resp, err := h.client.Get(h.url)
	if err != nil {
		result.Status = StatusFail
		result.Output = err.Error()
		return result, nil
	}
	defer resp.Body.Close()
	result.ObservedValue = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		result.Status = StatusFail
		result.Output = fmt.Sprintf("HTTP status: %d", resp.StatusCode)
	}
	return result, nil
}
