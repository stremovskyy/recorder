package redis_recorder

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/stremovskyy/recorder"
)

const (
	RequestPrefix  = "request"
	ResponsePrefix = "response"
	ErrorPrefix    = "error"
	MetricsPrefix  = "metrics"
	TagsPrefix     = "tag"
)

type redisRecorder struct {
	client     *redis.Client
	options    *Options
	compressor *compressor
	logger     *log.Logger
}

type asyncRecorder struct {
	*redisRecorder
}

var _ recorder.Recorder = (*redisRecorder)(nil)

type compressor struct {
	bufferPool sync.Pool
}

func newCompressor() *compressor {
	return &compressor{
		bufferPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

func (c *compressor) compressData(data []byte, lvl int) ([]byte, error) {
	buf := c.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer c.bufferPool.Put(buf)

	gz, err := gzip.NewWriterLevel(buf, lvl)
	if err != nil {
		return nil, err
	}
	_, err = gz.Write(data)
	if err != nil {
		gz.Close()
		return nil, err
	}
	err = gz.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *compressor) decompressData(compressedData []byte) ([]byte, error) {
	buf := c.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer c.bufferPool.Put(buf)

	_, err := buf.Write(compressedData)
	if err != nil {
		return nil, err
	}

	gz, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	decompressedData, err := io.ReadAll(gz)
	if err != nil {
		return nil, err
	}

	return decompressedData, nil
}

func NewRedisRecorder(options *Options) recorder.Recorder {
	client := redis.NewClient(
		&redis.Options{
			Addr:       options.Addr,
			Password:   options.Password,
			DB:         options.DB,
			ClientName: "RedisRecorder",
		},
	)

	if options.DefaultTTL == 0 {
		options.DefaultTTL = time.Hour * 24 * 7
	}

	if options.CompressionLvl == 0 {
		options.CompressionLvl = gzip.DefaultCompression
	}

	if options.Prefix == "" {
		options.Prefix = "RedisRecorder"
	}

	statusCmd := client.Ping(context.Background())
	if statusCmd.Err() != nil {
		log.Fatalf("failed to connect to redis server: %v", statusCmd.Err())
	}

	return &redisRecorder{
		client:     client,
		options:    options,
		compressor: newCompressor(),
		logger:     log.New(log.Writer(), "[Redis Recorder]: ", log.LstdFlags),
	}
}

func (r *redisRecorder) recordData(ctx context.Context, prefix, id, data string, compressedData []byte, tags map[string]string) error {
	key := fmt.Sprintf("%s:%s:%s", r.options.Prefix, prefix, id)
	if r.options.Debug {
		r.logger.Printf("%s key: %s", prefix, key)
	}

	if err := r.client.Set(ctx, key, compressedData, r.options.DefaultTTL).Err(); err != nil {
		return fmt.Errorf("failed to set %s data: %w", prefix, err)
	}

	return r.updateTagIndex(ctx, tags, key)
}

func (r *redisRecorder) record(ctx context.Context, prefix string, primaryID *string, id string, data []byte, tags map[string]string) error {
	if data == nil || len(data) == 0 {
		return fmt.Errorf("data cannot be nil or empty")
	}

	compressedData, err := r.compressor.compressData(data, r.options.CompressionLvl)
	if err != nil {
		return fmt.Errorf("failed to compress %s data: %w", prefix, err)
	}

	if primaryID != nil {
		id = fmt.Sprintf("%s:%s", *primaryID, id)
	}

	if tags == nil {
		tags = make(map[string]string)
	}
	tags["request_id"] = id

	return r.recordData(ctx, prefix, id, string(data), compressedData, tags)
}

func (r *redisRecorder) RecordRequest(ctx context.Context, primaryID *string, requestID string, request []byte, tags map[string]string) error {
	if requestID == "" {
		return fmt.Errorf("requestID cannot be empty")
	}
	return r.record(ctx, RequestPrefix, primaryID, requestID, request, tags)
}

func (r *redisRecorder) RecordResponse(ctx context.Context, primaryID *string, requestID string, response []byte, tags map[string]string) error {
	if requestID == "" {
		return fmt.Errorf("requestID cannot be empty")
	}
	return r.record(ctx, ResponsePrefix, primaryID, requestID, response, tags)
}

func (r *redisRecorder) RecordError(ctx context.Context, id *string, requestID string, err error, tags map[string]string) error {
	if err == nil {
		return fmt.Errorf("error cannot be nil")
	}
	if requestID == "" {
		return fmt.Errorf("requestID cannot be empty")
	}
	return r.record(ctx, ErrorPrefix, id, requestID, []byte(err.Error()), tags)
}

func (r *redisRecorder) RecordMetrics(ctx context.Context, primaryID *string, requestID string, metrics map[string]string, tags map[string]string) error {
	if requestID == "" {
		return fmt.Errorf("requestID cannot be empty")
	}
	if metrics == nil || len(metrics) == 0 {
		return fmt.Errorf("metrics cannot be nil or empty")
	}

	jsonData, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("cannot marshal metrics: %w", err)
	}
	return r.record(ctx, MetricsPrefix, primaryID, requestID, jsonData, tags)
}

func (r *redisRecorder) getData(ctx context.Context, prefix, id string) ([]byte, error) {
	key := fmt.Sprintf("%s:%s:%s", r.options.Prefix, prefix, id)
	data, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("%s data not found for id: %s", prefix, id)
		}
		return nil, fmt.Errorf("failed to get %s data: %w", prefix, err)
	}

	return r.compressor.decompressData([]byte(data))
}

func (r *redisRecorder) GetRequest(ctx context.Context, requestID string) ([]byte, error) {
	if requestID == "" {
		return nil, fmt.Errorf("requestID cannot be empty")
	}
	return r.getData(ctx, RequestPrefix, requestID)
}

func (r *redisRecorder) GetResponse(ctx context.Context, requestID string) ([]byte, error) {
	if requestID == "" {
		return nil, fmt.Errorf("requestID cannot be empty")
	}
	return r.getData(ctx, ResponsePrefix, requestID)
}

func (r *redisRecorder) FindByTag(ctx context.Context, tag string) ([]string, error) {
	if tag == "" {
		return nil, fmt.Errorf("tag cannot be empty")
	}

	tagKey := fmt.Sprintf("%s:%s:%s", r.options.Prefix, TagsPrefix, tag)
	tags, err := r.client.SMembers(ctx, tagKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to find by tag: %w", err)
	}
	return tags, nil
}

func (r *redisRecorder) Async() recorder.AsyncRecorder {
	return &asyncRecorder{r}
}

// Asynchronous Methods
func (ar *asyncRecorder) RecordRequest(ctx context.Context, primaryID *string, requestID string, request []byte, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.redisRecorder.RecordRequest(ctx, primaryID, requestID, request, tags)
	}()
	return result
}

func (ar *asyncRecorder) RecordResponse(ctx context.Context, primaryID *string, requestID string, response []byte, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.redisRecorder.RecordResponse(ctx, primaryID, requestID, response, tags)
	}()
	return result
}

func (ar *asyncRecorder) RecordError(ctx context.Context, id *string, requestID string, err error, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.redisRecorder.RecordError(ctx, id, requestID, err, tags)
	}()
	return result
}

func (ar *asyncRecorder) RecordMetrics(ctx context.Context, primaryID *string, requestID string, metrics map[string]string, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.redisRecorder.RecordMetrics(ctx, primaryID, requestID, metrics, tags)
	}()
	return result
}

func (ar *asyncRecorder) GetRequest(ctx context.Context, requestID string) <-chan recorder.Result {
	resultChan := make(chan recorder.Result, 1)
	go func() {
		data, err := ar.redisRecorder.GetRequest(ctx, requestID)
		resultChan <- recorder.Result{Data: data, Err: err}
	}()
	return resultChan
}

func (ar *asyncRecorder) GetResponse(ctx context.Context, requestID string) <-chan recorder.Result {
	resultChan := make(chan recorder.Result, 1)
	go func() {
		data, err := ar.redisRecorder.GetResponse(ctx, requestID)
		resultChan <- recorder.Result{Data: data, Err: err}
	}()
	return resultChan
}

func (ar *asyncRecorder) FindByTag(ctx context.Context, tag string) <-chan recorder.FindByTagResult {
	resultChan := make(chan recorder.FindByTagResult, 1)
	go func() {
		tags, err := ar.redisRecorder.FindByTag(ctx, tag)
		resultChan <- recorder.FindByTagResult{Tags: tags, Err: err}
	}()
	return resultChan
}

func (r *redisRecorder) updateTagIndex(ctx context.Context, tags map[string]string, itemKey string) error {
	for key, value := range tags {
		tagKey := fmt.Sprintf("%s:%s:%s:%s", r.options.Prefix, TagsPrefix, key, value)
		tagValue := itemKey

		if r.options.Debug {
			r.logger.Printf("Updating tag index: key=%s, value=%s", tagKey, tagValue)
		}

		_, err := r.client.SAdd(ctx, tagKey, tagValue).Result()
		if err != nil {
			return fmt.Errorf("failed to add tag to index for key %s: %w", tagKey, err)
		}

		_, err = r.client.Expire(ctx, tagKey, r.options.DefaultTTL).Result()
		if err != nil {
			r.logger.Printf("[ERROR] failed to set expiration for tag %s: %v", tagKey, err)
		}
	}

	return nil
}
