// client.go 实现 minio integration 的客户端封装与基础连接/调用能力。
package minio

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"time"

	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config 描述 MinIO 客户端的连接配置。
type Config struct {
	Name            string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
}

// Factory 定义 MinIO Client 的工厂接口。
type Factory interface {
	Default() *Client
	New(cfg Config) (*Client, error)
}

// Client 是对 `minio-go` 客户端的轻量封装。
type Client struct {
	client *miniosdk.Client
	cfg    Config
}

// New 基于配置创建一个 MinIO Client。
func New(cfg Config) (*Client, error) {
	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, err
	}
	return &Client{client: client, cfg: cfg}, nil
}

// Default 返回当前 Client 本身，用于满足 Factory 接口。
func (c *Client) Default() *Client                { return c }

// New 基于 cfg 创建一个新的 Client，用于满足 Factory 接口。
func (c *Client) New(cfg Config) (*Client, error) { return New(cfg) }

// Config 返回当前 Client 的配置副本。
func (c *Client) Config() Config                  { return c.cfg }

// Raw 返回底层 `*minio.Client`。
func (c *Client) Raw() *miniosdk.Client           { return c.client }

// MakeBucket 创建一个 bucket。
func (c *Client) MakeBucket(ctx context.Context, bucketName string, location string) error {
	return c.client.MakeBucket(ctx, bucketName, miniosdk.MakeBucketOptions{Region: location})
}

// BucketExists 检查 bucket 是否存在。
func (c *Client) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	return c.client.BucketExists(ctx, bucketName)
}

// RemoveBucket 删除一个 bucket。
func (c *Client) RemoveBucket(ctx context.Context, bucketName string) error {
	return c.client.RemoveBucket(ctx, bucketName)
}

// ListBuckets 列出当前账号可见的所有 bucket。
func (c *Client) ListBuckets(ctx context.Context) ([]miniosdk.BucketInfo, error) {
	return c.client.ListBuckets(ctx)
}

// PutObject 上传对象内容。
func (c *Client) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts miniosdk.PutObjectOptions) (miniosdk.UploadInfo, error) {
	return c.client.PutObject(ctx, bucketName, objectName, reader, objectSize, opts)
}

// PutObjectBytes 把字节切片作为对象内容上传。
func (c *Client) PutObjectBytes(ctx context.Context, bucketName, objectName string, data []byte, opts miniosdk.PutObjectOptions) (miniosdk.UploadInfo, error) {
	return c.client.PutObject(ctx, bucketName, objectName, bytes.NewReader(data), int64(len(data)), opts)
}

// GetObject 获取对象读取流。
func (c *Client) GetObject(ctx context.Context, bucketName, objectName string, opts miniosdk.GetObjectOptions) (*miniosdk.Object, error) {
	return c.client.GetObject(ctx, bucketName, objectName, opts)
}

// GetObjectBytes 读取对象全部内容并返回字节切片。
func (c *Client) GetObjectBytes(ctx context.Context, bucketName, objectName string) ([]byte, error) {
	object, err := c.client.GetObject(ctx, bucketName, objectName, miniosdk.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	return io.ReadAll(object)
}

// RemoveObject 删除一个对象。
func (c *Client) RemoveObject(ctx context.Context, bucketName, objectName string, opts miniosdk.RemoveObjectOptions) error {
	return c.client.RemoveObject(ctx, bucketName, objectName, opts)
}

// ListObjects 列出 bucket 下的对象流。
func (c *Client) ListObjects(ctx context.Context, bucketName string, opts miniosdk.ListObjectsOptions) <-chan miniosdk.ObjectInfo {
	return c.client.ListObjects(ctx, bucketName, opts)
}

// StatObject 获取对象元信息。
func (c *Client) StatObject(ctx context.Context, bucketName, objectName string, opts miniosdk.StatObjectOptions) (miniosdk.ObjectInfo, error) {
	return c.client.StatObject(ctx, bucketName, objectName, opts)
}

// CopyObject 在 MinIO 内部复制对象。
func (c *Client) CopyObject(ctx context.Context, dst miniosdk.CopyDestOptions, src miniosdk.CopySrcOptions) (miniosdk.UploadInfo, error) {
	return c.client.CopyObject(ctx, dst, src)
}

// FPutObject 从本地文件上传对象。
func (c *Client) FPutObject(ctx context.Context, bucketName, objectName, filePath string, opts miniosdk.PutObjectOptions) (miniosdk.UploadInfo, error) {
	return c.client.FPutObject(ctx, bucketName, objectName, filePath, opts)
}

// FGetObject 把对象下载到本地文件。
func (c *Client) FGetObject(ctx context.Context, bucketName, objectName, filePath string, opts miniosdk.GetObjectOptions) error {
	return c.client.FGetObject(ctx, bucketName, objectName, filePath, opts)
}

// PresignedGetObject 生成对象下载的预签名 URL。
func (c *Client) PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error) {
	return c.client.PresignedGetObject(ctx, bucketName, objectName, expires, reqParams)
}

// PresignedPutObject 生成对象上传的预签名 URL。
func (c *Client) PresignedPutObject(ctx context.Context, bucketName, objectName string, expires time.Duration) (*url.URL, error) {
	return c.client.PresignedPutObject(ctx, bucketName, objectName, expires)
}
