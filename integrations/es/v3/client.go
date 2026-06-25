// client.go 实现 es integration 的客户端封装与基础连接/调用能力。
package es

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Config 描述 Elasticsearch 客户端的连接配置。
type Config struct {
	Name      string
	Addresses []string
	Username  string
	Password  string
}

// Factory 定义 ES Client 的工厂接口。
type Factory interface {
	Default() *Client
	New(cfg Config) (*Client, error)
}

// Client 是对 Elasticsearch HTTP API 的轻量封装。
type Client struct {
	addresses []string
	username  string
	password  string
	http      *http.Client
}

// New 基于配置创建一个 ES Client。
func New(cfg Config) (*Client, error) {
	if len(cfg.Addresses) == 0 {
		cfg.Addresses = []string{"http://localhost:9200"}
	}
	return &Client{
		addresses: append([]string{}, cfg.Addresses...),
		username:  cfg.Username,
		password:  cfg.Password,
		http:      &http.Client{},
	}, nil
}

// Default 返回当前 Client 本身，用于满足 Factory 接口。
func (c *Client) Default() *Client                { return c }

// New 基于 cfg 创建一个新的 Client，用于满足 Factory 接口。
func (c *Client) New(cfg Config) (*Client, error) { return New(cfg) }

// Config 返回当前 Client 的配置副本。
func (c *Client) Config() Config {
	return Config{Addresses: append([]string{}, c.addresses...), Username: c.username, Password: c.password}
}

// Addresses 返回地址列表副本。
func (c *Client) Addresses() []string { return append([]string{}, c.addresses...) }

// GetAddress 返回默认请求地址。
func (c *Client) GetAddress() string { return c.addresses[0] }

// DoRequest 发送一个原始 ES HTTP 请求；当 body 非空时会先编码为 JSON。
func (c *Client) DoRequest(method, path string, body interface{}) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.GetAddress()+path, reader)
	if err != nil {
		return nil, err
	}
	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.http.Do(req)
}

// Repository 是一个面向单索引的泛型仓储封装。
type Repository[T any] struct {
	client *Client
	index  string
}

// NewRepository 基于 client 和索引名创建一个泛型仓储。
func NewRepository[T any](client *Client, index string) *Repository[T] {
	return &Repository[T]{client: client, index: index}
}

// Index 使用指定 id 写入或覆盖一个文档。
func (r *Repository[T]) Index(id string, doc *T) error {
	_, err := r.client.DoRequest("PUT", fmt.Sprintf("/%s/_doc/%s", r.index, id), doc)
	return err
}

// Create 创建一个文档，并返回 ES 生成的 id。
func (r *Repository[T]) Create(doc *T) (string, error) {
	resp, err := r.client.DoRequest("POST", fmt.Sprintf("/%s/_doc", r.index), doc)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if id, ok := result["_id"].(string); ok {
		return id, nil
	}
	return "", fmt.Errorf("no id in response")
}

// Get 按 id 读取一个文档。
func (r *Repository[T]) Get(id string) (*T, error) {
	resp, err := r.client.DoRequest("GET", fmt.Sprintf("/%s/_doc/%s", r.index, id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("document not found")
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	source, ok := result["_source"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response")
	}
	data, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	var doc T
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// Search 执行一次查询，并把命中的 `_source` 解码为 `[]*T`。
func (r *Repository[T]) Search(query interface{}) ([]*T, error) {
	resp, err := r.client.DoRequest("POST", fmt.Sprintf("/%s/_search", r.index), query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	var docs []*T
	if hits, ok := result["hits"].(map[string]interface{}); ok {
		if hitArray, ok := hits["hits"].([]interface{}); ok {
			for _, hit := range hitArray {
				hitMap, ok := hit.(map[string]interface{})
				if !ok {
					continue
				}
				source, ok := hitMap["_source"].(map[string]interface{})
				if !ok {
					continue
				}
				data, err := json.Marshal(source)
				if err != nil {
					continue
				}
				var doc T
				if err := json.Unmarshal(data, &doc); err != nil {
					continue
				}
				docs = append(docs, &doc)
			}
		}
	}
	return docs, nil
}

// MatchAll 执行 `match_all` 查询。
func (r *Repository[T]) MatchAll() ([]*T, error) {
	return r.Search(map[string]interface{}{"query": map[string]interface{}{"match_all": map[string]interface{}{}}})
}

// Exists 检查指定 id 的文档是否存在。
func (r *Repository[T]) Exists(id string) (bool, error) {
	resp, err := r.client.DoRequest("HEAD", fmt.Sprintf("/%s/_doc/%s", r.index, id), nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// BulkIndex 以 bulk API 批量写入文档。
func (r *Repository[T]) BulkIndex(docs map[string]*T) error {
	if len(docs) == 0 {
		return nil
	}
	var sb strings.Builder
	for id, doc := range docs {
		meta, _ := json.Marshal(map[string]interface{}{"index": map[string]interface{}{"_id": id}})
		sb.Write(meta)
		sb.WriteByte('\n')
		body, _ := json.Marshal(doc)
		sb.Write(body)
		sb.WriteByte('\n')
	}
	req, err := http.NewRequest("POST", r.client.GetAddress()+fmt.Sprintf("/%s/_bulk", r.index), strings.NewReader(sb.String()))
	if err != nil {
		return err
	}
	if r.client.username != "" && r.client.password != "" {
		req.SetBasicAuth(r.client.username, r.client.password)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	resp, err := r.client.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
