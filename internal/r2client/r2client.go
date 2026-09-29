package r2client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const DefaultMaxBucketBytes int64 = 10 * 1024 * 1024 * 1024

type Config struct {
	AccountID       string
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	PublicBaseURL   string
	MaxBucketBytes  int64
}

type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

type UploadFile struct {
	Key  string
	Path string
}

type Client struct {
	cfg Config
	s3  *s3.Client
}

func FromEnv() Config {
	return Config{
		AccountID:       os.Getenv("R2_ACCOUNT_ID"),
		Endpoint:        os.Getenv("R2_ENDPOINT"),
		Bucket:          os.Getenv("R2_BUCKET_NAME"),
		AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		PublicBaseURL:   os.Getenv("R2_PUBLIC_BASE_URL"),
		MaxBucketBytes:  DefaultMaxBucketBytes,
	}
}

func (c Config) WithDefaults() Config {
	c.AccountID = strings.TrimSpace(c.AccountID)
	c.Endpoint = strings.TrimSpace(c.Endpoint)
	c.Bucket = strings.TrimSpace(c.Bucket)
	c.AccessKeyID = strings.TrimSpace(c.AccessKeyID)
	c.SecretAccessKey = strings.TrimSpace(c.SecretAccessKey)
	c.PublicBaseURL = strings.TrimSpace(c.PublicBaseURL)

	if c.Endpoint == "" && c.AccountID != "" {
		c.Endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", c.AccountID)
	}
	if c.MaxBucketBytes <= 0 {
		c.MaxBucketBytes = DefaultMaxBucketBytes
	}
	return c
}

func (c Config) Validate() error {
	c = c.WithDefaults()
	var missing []string
	if c.Endpoint == "" {
		missing = append(missing, "R2_ENDPOINT ou R2_ACCOUNT_ID")
	}
	if c.Bucket == "" {
		missing = append(missing, "R2_BUCKET_NAME")
	}
	if c.AccessKeyID == "" {
		missing = append(missing, "R2_ACCESS_KEY_ID")
	}
	if c.SecretAccessKey == "" {
		missing = append(missing, "R2_SECRET_ACCESS_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("configuracao R2 incompleta: %s", strings.Join(missing, ", "))
	}
	return nil
}

func New(ctx context.Context, c Config) (*Client, error) {
	c = c.WithDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("auto"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(c.Endpoint)
		o.UsePathStyle = true
	})
	return &Client{cfg: c, s3: s3Client}, nil
}

func (c *Client) PublicURL(key string) string {
	base := strings.TrimRight(c.cfg.PublicBaseURL, "/")
	if base == "" {
		return ""
	}
	escaped := (&url.URL{Path: strings.TrimLeft(key, "/")}).EscapedPath()
	return base + "/" + escaped
}

func (c *Client) ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, int64, error) {
	var objects []ObjectInfo
	var total int64

	input := &s3.ListObjectsV2Input{Bucket: aws.String(c.cfg.Bucket)}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	paginator := s3.NewListObjectsV2Paginator(c.s3, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, 0, err
		}
		for _, item := range page.Contents {
			key := aws.ToString(item.Key)
			if key == "" {
				continue
			}
			size := aws.ToInt64(item.Size)
			total += size
			modified := time.Time{}
			if item.LastModified != nil {
				modified = *item.LastModified
			}
			objects = append(objects, ObjectInfo{Key: key, Size: size, LastModified: modified})
		}
	}
	return objects, total, nil
}

func (c *Client) PutFile(ctx context.Context, key string, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return c.put(ctx, key, file, contentType)
}

func (c *Client) PutBytes(ctx context.Context, key string, data []byte, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return c.put(ctx, key, bytes.NewReader(data), contentType)
}

func (c *Client) put(ctx context.Context, key string, body io.Reader, contentType string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("chave R2 vazia")
	}
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.cfg.Bucket),
		Key:         aws.String(strings.TrimLeft(key, "/")),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	return err
}

func (c *Client) DeleteObjects(ctx context.Context, keys []string) error {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(c.cfg.Bucket),
			Key:    aws.String(key),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) EnsureRoom(ctx context.Context, incoming map[string]int64, allowDeleteOldReleases bool) error {
	objects, total, err := c.ListObjects(ctx, "")
	if err != nil {
		return err
	}

	existing := make(map[string]int64, len(objects))
	for _, object := range objects {
		existing[object.Key] = object.Size
	}

	projected := total
	for key, size := range incoming {
		projected -= existing[key]
		projected += size
	}
	if projected <= c.cfg.MaxBucketBytes {
		return nil
	}
	if !allowDeleteOldReleases {
		return fmt.Errorf("upload bloqueado: R2 projetado em %.2f GB, limite %.2f GB", gb(projected), gb(c.cfg.MaxBucketBytes))
	}

	protected := make(map[string]bool, len(incoming)+1)
	protected["latest.json"] = true
	for key := range incoming {
		protected[key] = true
	}

	releases := make([]ObjectInfo, 0)
	for _, object := range objects {
		if protected[object.Key] || !IsReleaseKey(object.Key) {
			continue
		}
		releases = append(releases, object)
	}
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].LastModified.Before(releases[j].LastModified)
	})

	var deleteKeys []string
	for _, release := range releases {
		deleteKeys = append(deleteKeys, release.Key)
		projected -= release.Size
		if projected <= c.cfg.MaxBucketBytes {
			break
		}
	}
	if projected > c.cfg.MaxBucketBytes {
		return fmt.Errorf("upload bloqueado: mesmo removendo releases antigos, R2 ficaria em %.2f GB de %.2f GB", gb(projected), gb(c.cfg.MaxBucketBytes))
	}
	return c.DeleteObjects(ctx, deleteKeys)
}

// IsReleaseKey reconhece os arquivos de release no bucket, para a limpeza saber
// o que pode apagar quando o espaco aperta. Nao olha o nome do app: o bucket tem
// os arquivos antigos (OSInfotech-*) e os novos (DBS-assistencia-*) ao mesmo
// tempo enquanto os clientes nao atualizam.
func IsReleaseKey(key string) bool {
	name := pathBase(key)
	return (strings.Contains(name, "-Setup-") && strings.HasSuffix(name, ".exe")) ||
		(strings.Contains(name, "-Portable-") && strings.HasSuffix(name, ".zip")) ||
		(strings.Contains(name, "-Android-") && strings.HasSuffix(name, ".apk"))
}

func FileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func gb(bytes int64) float64 {
	return float64(bytes) / 1024 / 1024 / 1024
}

func pathBase(key string) string {
	key = strings.TrimRight(key, "/")
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[i+1:]
	}
	return key
}
