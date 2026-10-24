package usecase

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/config"
	"github.com/chaitin/MonkeyCode/backend/domain"
	"github.com/chaitin/MonkeyCode/backend/pkg/oss"
)

// errStoreDisabled 在对象存储未启用时返回，Presign 与 digestObject 把它翻译成
// 快照同步失败业务码。
var errStoreDisabled = errors.New("object storage is disabled")

// ossStore 把 *oss.Client 适配成 domain.SnapshotStore。oss.Client 的 Presign
// 返回自己的类型，这里转成两个字符串，避免 domain 依赖 pkg/oss。
type ossStore struct {
	c *oss.Client
}

func (s *ossStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if s.c == nil {
		return nil, errStoreDisabled
	}
	return s.c.GetObject(ctx, key)
}

func (s *ossStore) PresignPut(ctx context.Context, prefix, filename string, expires time.Duration) (string, string, error) {
	if s.c == nil {
		return "", "", errStoreDisabled
	}
	p, err := s.c.Presign(ctx, prefix, filename, expires)
	if err != nil {
		return "", "", err
	}
	return p.UploadURL, p.AccessURL, nil
}

// NewSnapshotStore 创建对象存储适配器。对象存储未启用时返回空实现而不是 nil，
// 保证 DI 容器里永远有可解析的值。
func NewSnapshotStore(i *do.Injector) (domain.SnapshotStore, error) {
	cfg := do.MustInvoke[*config.Config](i)
	if !cfg.ObjectStorage.Enabled {
		return &ossStore{}, nil
	}
	opt := oss.S3Option{
		ForcePathStyle: cfg.ObjectStorage.ForcePathStyle,
		InitBucket:     cfg.ObjectStorage.InitBucket,
	}
	c, err := oss.NewS3Compatible(context.Background(), cfg.ObjectStorage, opt)
	if err != nil {
		return nil, err
	}
	return &ossStore{c: c.WithAccessEndpoint(cfg.ObjectStorage.AccessEndpoint)}, nil
}
