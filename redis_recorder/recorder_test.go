package redis_recorder

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/stremovskyy/recorder"
)

func newTestRedisRecorder(t *testing.T) (*redisRecorder, recorder.Recorder, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	opts := &Options{
		Addr:            mr.Addr(),
		DefaultTTL:      time.Hour,
		CompressionLvl:  0,
		Prefix:          "testrc",
		MaxRetries:      1,
		MinRetryBackoff: time.Millisecond,
		MaxRetryBackoff: 10 * time.Millisecond,
		DialTimeout:     time.Second,
		ReadTimeout:     time.Second,
		WriteTimeout:    time.Second,
		PoolSize:        5,
		MinIdleConns:    1,
		MaxConnAge:      time.Minute,
		PoolTimeout:     time.Second,
		IdleTimeout:     2 * time.Second,
	}

	client := redis.NewClient(&redis.Options{
		Addr:            opts.Addr,
		Password:        opts.Password,
		DB:              opts.DB,
		MaxRetries:      opts.MaxRetries,
		MinRetryBackoff: opts.MinRetryBackoff,
		MaxRetryBackoff: opts.MaxRetryBackoff,
		DialTimeout:     opts.DialTimeout,
		ReadTimeout:     opts.ReadTimeout,
		WriteTimeout:    opts.WriteTimeout,
		PoolSize:        opts.PoolSize,
		MinIdleConns:    opts.MinIdleConns,
		ConnMaxLifetime: opts.MaxConnAge,
		ConnMaxIdleTime: opts.IdleTimeout,
		PoolTimeout:     opts.PoolTimeout,
	})

	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})

	storage := &redisRecorder{
		client:     client,
		options:    opts,
		compressor: newCompressor(),
		logger:     recorder.NewDefaultLogger().With("component", "redis_test"),
		metrics:    recorder.NewMetrics(),
	}

	return storage, recorder.New(storage), mr
}

func TestRedisRecorderRoundTrip(t *testing.T) {
	storage, rec, _ := newTestRedisRecorder(t)

	ctx := context.Background()

	if err := rec.RecordRequest(ctx, nil, "req1", []byte("request-data"), map[string]string{"env": "dev"}); err != nil {
		t.Fatalf("RecordRequest returned error: %v", err)
	}

	data, err := rec.GetRequest(ctx, "req1")
	if err != nil {
		t.Fatalf("GetRequest returned error: %v", err)
	}
	if string(data) != "request-data" {
		t.Fatalf("expected request payload, got %s", string(data))
	}

	async := rec.Async()
	if err := <-async.RecordResponse(ctx, nil, "req1", []byte("response-data"), map[string]string{"env": "dev"}); err != nil {
		t.Fatalf("async RecordResponse returned error: %v", err)
	}

	responseResult := <-async.GetResponse(ctx, "req1")
	if responseResult.Err != nil {
		t.Fatalf("GetResponse returned error: %v", responseResult.Err)
	}
	if string(responseResult.Data) != "response-data" {
		t.Fatalf("expected response payload, got %s", string(responseResult.Data))
	}

	if err := rec.RecordError(ctx, nil, "req1", errors.New("boom"), map[string]string{"severity": "high"}); err != nil {
		t.Fatalf("RecordError returned error: %v", err)
	}

	if err := rec.RecordMetrics(ctx, nil, "req1", map[string]string{"duration": "10"}, map[string]string{"env": "dev"}); err != nil {
		t.Fatalf("RecordMetrics returned error: %v", err)
	}

	// Ensure tag index works for custom tags and automatic ones.
	tags, err := rec.FindByTag(ctx, "env:dev")
	if err != nil {
		t.Fatalf("FindByTag returned error: %v", err)
	}
	expectedRequestKey := fmt.Sprintf("%s:%s:%s", storage.options.Prefix, RequestPrefix, "req1")
	expectedResponseKey := fmt.Sprintf("%s:%s:%s", storage.options.Prefix, ResponsePrefix, "req1")
	expectedMetricsKey := fmt.Sprintf("%s:%s:%s", storage.options.Prefix, MetricsPrefix, "req1")

	for _, expected := range []string{expectedRequestKey, expectedResponseKey, expectedMetricsKey} {
		if !contains(tags, expected) {
			t.Fatalf("expected env:dev tags to contain %s, got %v", expected, tags)
		}
	}

	recordTypeTags, err := rec.FindByTag(ctx, "record_type:response")
	if err != nil {
		t.Fatalf("FindByTag returned error: %v", err)
	}
	if !contains(recordTypeTags, expectedResponseKey) {
		t.Fatalf("expected record_type:response to include %s, got %v", expectedResponseKey, recordTypeTags)
	}

	requestIDTags, err := rec.FindByTag(ctx, "request_id:req1")
	if err != nil {
		t.Fatalf("FindByTag returned error: %v", err)
	}
	if !contains(requestIDTags, expectedResponseKey) || !contains(requestIDTags, expectedRequestKey) {
		t.Fatalf("expected request_id tags to include request and response keys, got %v", requestIDTags)
	}
}

func TestCompressorReuseSafety(t *testing.T) {
	comp := newCompressor()
	firstCompressed, err := comp.compressData([]byte("first"), gzip.NoCompression)
	if err != nil {
		t.Fatalf("compress first: %v", err)
	}
	secondCompressed, err := comp.compressData([]byte("second"), gzip.NoCompression)
	if err != nil {
		t.Fatalf("compress second: %v", err)
	}

	firstDecompressed, err := comp.decompressData(firstCompressed)
	if err != nil {
		t.Fatalf("decompress first: %v", err)
	}
	secondDecompressed, err := comp.decompressData(secondCompressed)
	if err != nil {
		t.Fatalf("decompress second: %v", err)
	}

	if string(firstDecompressed) != "first" {
		t.Fatalf("expected first payload, got %s", string(firstDecompressed))
	}
	if string(secondDecompressed) != "second" {
		t.Fatalf("expected second payload, got %s", string(secondDecompressed))
	}
	if len(firstCompressed) == 0 || len(secondCompressed) == 0 {
		t.Fatal("expected compressed payloads to be non-empty")
	}
	if &firstCompressed[0] == &secondCompressed[0] {
		t.Fatalf("expected independent compressed buffers")
	}
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func TestRedisRecorderPrefixForUnknown(t *testing.T) {
	storage, _, _ := newTestRedisRecorder(t)
	if _, err := storage.prefixFor(recorder.RecordType("invalid")); err == nil {
		t.Fatal("expected error for unknown record type")
	}
}

func TestNewRedisRecorderWithValidationDefaults(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	opts := &Options{Addr: mr.Addr()}
	rec, err := NewRedisRecorderWithValidation(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.DefaultTTL == 0 {
		t.Fatal("expected default TTL to be set")
	}
	if opts.CompressionLvl == 0 {
		t.Fatal("expected compression level default")
	}
	if opts.Prefix == "" {
		t.Fatal("expected prefix default")
	}

	// Exercise the returned recorder minimally to ensure it functions.
	if err := rec.RecordRequest(context.Background(), nil, "ping", []byte("pong"), nil); err != nil {
		t.Fatalf("RecordRequest returned error: %v", err)
	}
}

func TestNewRedisRecorderWithValidationErrors(t *testing.T) {
	if _, err := NewRedisRecorderWithValidation(nil); err == nil {
		t.Fatal("expected error when options are nil")
	}

	if _, err := NewRedisRecorderWithValidation(&Options{}); err == nil {
		t.Fatal("expected error for missing address")
	}
}

func TestNewRedisRecorderGracefulFailure(t *testing.T) {
	if rec := NewRedisRecorder(nil); rec != nil {
		t.Fatal("expected nil recorder when options invalid")
	}
}
