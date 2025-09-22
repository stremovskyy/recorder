package redis_recorder

import (
	"bytes"
	"compress/gzip"
	"context"
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
	logger     recorder.Logger
	metrics    recorder.Metrics
}

var _ recorder.Storage = (*redisRecorder)(nil)

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

	compressed := buf.Bytes()
	result := make([]byte, len(compressed))
	copy(result, compressed)
	return result, nil
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
	rec, err := NewRedisRecorderWithValidation(options)
	if err != nil {
		log.Printf("failed to create redis recorder: %v", err)
		return nil
	}
	return rec
}

func NewRedisRecorderWithValidation(options *Options) (recorder.Recorder, error) {
	if options == nil {
		return nil, fmt.Errorf("redis recorder: options must not be nil")
	}

	applyRedisDefaults(options)
	if err := options.Validate(); err != nil {
		return nil, err
	}
	cfg := *options

	client := redis.NewClient(
		&redis.Options{
			Addr:            cfg.Addr,
			Password:        cfg.Password,
			DB:              cfg.DB,
			ClientName:      "RedisRecorder",
			MaxRetries:      cfg.MaxRetries,
			MinRetryBackoff: cfg.MinRetryBackoff,
			MaxRetryBackoff: cfg.MaxRetryBackoff,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			ConnMaxLifetime: cfg.MaxConnAge,
			PoolTimeout:     cfg.PoolTimeout,
			ConnMaxIdleTime: cfg.IdleTimeout,
		},
	)

	// Only validate connection for the new function, not backward compatible one
	// Skip ping test for backward compatibility - let it fail at runtime if needed

	logger := recorder.NewDefaultLogger().With("component", "redis_recorder")
	metrics := recorder.NewMetrics()

	storage := &redisRecorder{
		client:     client,
		options:    options,
		compressor: newCompressor(),
		logger:     logger,
		metrics:    metrics,
	}

	return recorder.New(storage), nil
}

func applyRedisDefaults(options *Options) {
	if options.DefaultTTL == 0 {
		options.DefaultTTL = time.Hour * 24 * 7
	}
	if options.CompressionLvl == 0 {
		options.CompressionLvl = gzip.DefaultCompression
	}
	if options.Prefix == "" {
		options.Prefix = "RedisRecorder"
	}
	if options.DialTimeout == 0 {
		options.DialTimeout = 5 * time.Second
	}
	if options.ReadTimeout == 0 {
		options.ReadTimeout = 3 * time.Second
	}
	if options.WriteTimeout == 0 {
		options.WriteTimeout = 3 * time.Second
	}
	if options.PoolSize == 0 {
		options.PoolSize = 10
	}
	if options.MaxRetries == 0 {
		options.MaxRetries = 3
	}
	if options.MinRetryBackoff == 0 {
		options.MinRetryBackoff = 8 * time.Millisecond
	}
	if options.MaxRetryBackoff == 0 {
		options.MaxRetryBackoff = 512 * time.Millisecond
	}
	if options.PoolTimeout == 0 {
		options.PoolTimeout = 4 * time.Second
	}
	if options.IdleTimeout == 0 {
		options.IdleTimeout = 5 * time.Minute
	}
	if options.MaxConnAge == 0 {
		options.MaxConnAge = 30 * time.Minute
	}
}

func (r *redisRecorder) Save(ctx context.Context, record recorder.Record) error {
	if record.RequestID == "" {
		return fmt.Errorf("requestID cannot be empty")
	}
	if len(record.Payload) == 0 {
		return fmt.Errorf("data cannot be nil or empty")
	}

	prefix, err := r.prefixFor(record.Type)
	if err != nil {
		return err
	}

	compressedData, err := r.compressor.compressData(record.Payload, r.options.CompressionLvl)
	if err != nil {
		return fmt.Errorf("failed to compress %s data: %w", prefix, err)
	}

	id := record.RequestID
	if record.PrimaryID != nil && *record.PrimaryID != "" {
		id = fmt.Sprintf("%s:%s", *record.PrimaryID, id)
	}

	tags := record.Tags
	if tags == nil {
		tags = make(map[string]string)
	}
	tags["request_id"] = id
	tags["record_type"] = prefix

	return r.recordData(ctx, prefix, id, compressedData, tags)
}

func (r *redisRecorder) Load(ctx context.Context, recordType recorder.RecordType, requestID string) ([]byte, error) {
	if requestID == "" {
		return nil, fmt.Errorf("requestID cannot be empty")
	}

	prefix, err := r.prefixFor(recordType)
	if err != nil {
		return nil, err
	}

	return r.getData(ctx, prefix, requestID)
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

func (r *redisRecorder) prefixFor(recordType recorder.RecordType) (string, error) {
	switch recordType {
	case recorder.RecordTypeRequest:
		return RequestPrefix, nil
	case recorder.RecordTypeResponse:
		return ResponsePrefix, nil
	case recorder.RecordTypeError:
		return ErrorPrefix, nil
	case recorder.RecordTypeMetrics:
		return MetricsPrefix, nil
	default:
		return "", fmt.Errorf("unknown record type: %s", recordType)
	}
}

func (r *redisRecorder) recordData(ctx context.Context, prefix, id string, compressedData []byte, tags map[string]string) error {
	start := time.Now()
	defer func() {
		r.metrics.RecordTiming("redis.record_data.duration", time.Since(start), map[string]string{"prefix": prefix})
	}()

	key := fmt.Sprintf("%s:%s:%s", r.options.Prefix, prefix, id)
	logger := r.logger.WithContext(ctx).With("prefix", prefix, "key", key)

	if r.options.Debug {
		logger.Debug("recording data", "data_size", len(compressedData))
	}

	if err := r.client.Set(ctx, key, compressedData, r.options.DefaultTTL).Err(); err != nil {
		r.metrics.IncrementCounter("redis.record_data.errors", map[string]string{"prefix": prefix, "error": "set_failed"})
		logger.Error("failed to set data", "error", err)
		return fmt.Errorf("failed to set %s data: %w", prefix, err)
	}

	r.metrics.IncrementCounter("redis.record_data.success", map[string]string{"prefix": prefix})
	logger.Debug("data recorded successfully")
	return r.updateTagIndex(ctx, tags, key)
}

func (r *redisRecorder) getData(ctx context.Context, prefix, id string) ([]byte, error) {
	start := time.Now()
	defer func() {
		r.metrics.RecordTiming("redis.get_data.duration", time.Since(start), map[string]string{"prefix": prefix})
	}()

	key := fmt.Sprintf("%s:%s:%s", r.options.Prefix, prefix, id)
	logger := r.logger.WithContext(ctx).With("prefix", prefix, "key", key)

	data, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			r.metrics.IncrementCounter("redis.get_data.not_found", map[string]string{"prefix": prefix})
			logger.Warn("data not found", "id", id)
			return nil, fmt.Errorf("%s data not found for id: %s", prefix, id)
		}
		r.metrics.IncrementCounter("redis.get_data.errors", map[string]string{"prefix": prefix, "error": "get_failed"})
		logger.Error("failed to get data", "error", err)
		return nil, fmt.Errorf("failed to get %s data: %w", prefix, err)
	}

	decompressedData, err := r.compressor.decompressData([]byte(data))
	if err != nil {
		r.metrics.IncrementCounter("redis.get_data.errors", map[string]string{"prefix": prefix, "error": "decompress_failed"})
		logger.Error("failed to decompress data", "error", err)
		return nil, fmt.Errorf("failed to decompress %s data: %w", prefix, err)
	}

	r.metrics.IncrementCounter("redis.get_data.success", map[string]string{"prefix": prefix})
	logger.Debug("data retrieved successfully", "data_size", len(decompressedData))
	return decompressedData, nil
}

func (r *redisRecorder) updateTagIndex(ctx context.Context, tags map[string]string, itemKey string) error {
	for key, value := range tags {
		tagKey := fmt.Sprintf("%s:%s:%s:%s", r.options.Prefix, TagsPrefix, key, value)
		tagValue := itemKey

		logger := r.logger.WithContext(ctx).With("tag_key", tagKey, "tag_value", tagValue)

		if r.options.Debug {
			logger.Debug("updating tag index")
		}

		_, err := r.client.SAdd(ctx, tagKey, tagValue).Result()
		if err != nil {
			r.metrics.IncrementCounter("redis.tag_index.errors", map[string]string{"operation": "sadd"})
			logger.Error("failed to add tag to index", "error", err)
			return fmt.Errorf("failed to add tag to index for key %s: %w", tagKey, err)
		}

		_, err = r.client.Expire(ctx, tagKey, r.options.DefaultTTL).Result()
		if err != nil {
			r.metrics.IncrementCounter("redis.tag_index.errors", map[string]string{"operation": "expire"})
			logger.Error("failed to set expiration for tag", "error", err)
		}
	}

	return nil
}
