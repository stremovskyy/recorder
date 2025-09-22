package redis_recorder

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Options struct {
	Debug           bool
	Addr            string
	Password        string
	DB              int
	DefaultTTL      time.Duration
	CompressionLvl  int
	Prefix          string
	MaxRetries      int
	MinRetryBackoff time.Duration
	MaxRetryBackoff time.Duration
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	PoolSize        int
	MinIdleConns    int
	MaxConnAge      time.Duration
	PoolTimeout     time.Duration
	IdleTimeout     time.Duration
}

func NewDefaultOptions(addr string, password string, DB int) *Options {
	return &Options{
		Addr:            addr,
		Password:        password,
		DB:              DB,
		DefaultTTL:      24 * time.Hour * 7, // one week
		CompressionLvl:  3,
		Prefix:          "recorder",
		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		PoolSize:        10,
		MinIdleConns:    2,
		MaxConnAge:      30 * time.Minute,
		PoolTimeout:     4 * time.Second,
		IdleTimeout:     5 * time.Minute,
	}
}

func NewOptionsFromEnv() *Options {
	opts := &Options{
		Addr:            getEnvOrDefault("REDIS_ADDR", "localhost:6379"),
		Password:        os.Getenv("REDIS_PASSWORD"),
		DB:              getEnvIntOrDefault("REDIS_DB", 0),
		DefaultTTL:      getEnvDurationOrDefault("REDIS_DEFAULT_TTL", 24*time.Hour*7),
		CompressionLvl:  getEnvIntOrDefault("REDIS_COMPRESSION_LEVEL", 3),
		Prefix:          getEnvOrDefault("REDIS_PREFIX", "recorder"),
		MaxRetries:      getEnvIntOrDefault("REDIS_MAX_RETRIES", 3),
		MinRetryBackoff: getEnvDurationOrDefault("REDIS_MIN_RETRY_BACKOFF", 8*time.Millisecond),
		MaxRetryBackoff: getEnvDurationOrDefault("REDIS_MAX_RETRY_BACKOFF", 512*time.Millisecond),
		DialTimeout:     getEnvDurationOrDefault("REDIS_DIAL_TIMEOUT", 5*time.Second),
		ReadTimeout:     getEnvDurationOrDefault("REDIS_READ_TIMEOUT", 3*time.Second),
		WriteTimeout:    getEnvDurationOrDefault("REDIS_WRITE_TIMEOUT", 3*time.Second),
		PoolSize:        getEnvIntOrDefault("REDIS_POOL_SIZE", 10),
		MinIdleConns:    getEnvIntOrDefault("REDIS_MIN_IDLE_CONNS", 2),
		MaxConnAge:      getEnvDurationOrDefault("REDIS_MAX_CONN_AGE", 30*time.Minute),
		PoolTimeout:     getEnvDurationOrDefault("REDIS_POOL_TIMEOUT", 4*time.Second),
		IdleTimeout:     getEnvDurationOrDefault("REDIS_IDLE_TIMEOUT", 5*time.Minute),
		Debug:           getEnvBoolOrDefault("REDIS_DEBUG", false),
	}
	return opts
}

func (o *Options) Validate() error {
	if o.Addr == "" {
		return fmt.Errorf("redis address cannot be empty")
	}
	if o.DefaultTTL <= 0 {
		return fmt.Errorf("default TTL must be positive")
	}
	if o.CompressionLvl < -1 || o.CompressionLvl > 9 {
		return fmt.Errorf("compression level must be between -1 and 9")
	}
	if o.Prefix == "" {
		return fmt.Errorf("prefix cannot be empty")
	}
	if o.PoolSize <= 0 {
		o.PoolSize = 10 // default pool size
	}
	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
