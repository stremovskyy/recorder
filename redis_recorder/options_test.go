package redis_recorder

import (
	"testing"
	"time"
)

func TestOptionsValidate(t *testing.T) {
	o := &Options{}
	if err := o.Validate(); err == nil {
		t.Fatal("expected error when address is empty")
	}

	o = &Options{Addr: "localhost:6379", DefaultTTL: time.Second, CompressionLvl: 10, Prefix: "test", PoolSize: 1}
	if err := o.Validate(); err == nil {
		t.Fatal("expected error for invalid compression level")
	}

	o = &Options{Addr: "localhost:6379", DefaultTTL: time.Second, CompressionLvl: 1, Prefix: "test", PoolSize: 0}
	if err := o.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if o.PoolSize != 10 {
		t.Fatalf("expected pool size default of 10, got %d", o.PoolSize)
	}
}

func TestNewOptionsFromEnv(t *testing.T) {
	t.Setenv("REDIS_ADDR", "127.0.0.1:9999")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("REDIS_DEFAULT_TTL", "30s")
	t.Setenv("REDIS_COMPRESSION_LEVEL", "4")
	t.Setenv("REDIS_PREFIX", "custom")
	t.Setenv("REDIS_MAX_RETRIES", "5")
	t.Setenv("REDIS_MIN_RETRY_BACKOFF", "1ms")
	t.Setenv("REDIS_MAX_RETRY_BACKOFF", "2ms")
	t.Setenv("REDIS_DIAL_TIMEOUT", "100ms")
	t.Setenv("REDIS_READ_TIMEOUT", "200ms")
	t.Setenv("REDIS_WRITE_TIMEOUT", "300ms")
	t.Setenv("REDIS_POOL_SIZE", "20")
	t.Setenv("REDIS_MIN_IDLE_CONNS", "3")
	t.Setenv("REDIS_MAX_CONN_AGE", "1m")
	t.Setenv("REDIS_POOL_TIMEOUT", "400ms")
	t.Setenv("REDIS_IDLE_TIMEOUT", "500ms")
	t.Setenv("REDIS_DEBUG", "true")

	opts := NewOptionsFromEnv()
	if opts.Addr != "127.0.0.1:9999" {
		t.Fatalf("unexpected addr: %s", opts.Addr)
	}
	if opts.DB != 2 {
		t.Fatalf("unexpected db: %d", opts.DB)
	}
	if opts.DefaultTTL != 30*time.Second {
		t.Fatalf("unexpected ttl: %s", opts.DefaultTTL)
	}
	if !opts.Debug {
		t.Fatal("expected debug to be true")
	}
}
