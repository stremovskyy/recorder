package recorder

import (
	"context"
)

type Result struct {
	Data []byte
	Err  error
}

type FindByTagResult struct {
	Tags []string
	Err  error
}

// Recorder is the public interface for the redisRecorder.
type Recorder interface {
	RecordRequest(ctx context.Context, primaryID *string, requestID string, request []byte, tags map[string]string) error
	RecordResponse(ctx context.Context, primaryID *string, requestID string, response []byte, tags map[string]string) error
	RecordError(ctx context.Context, id *string, requestID string, err error, tags map[string]string) error
	RecordMetrics(ctx context.Context, primaryID *string, requestID string, metrics map[string]string, tags map[string]string) error
	GetRequest(ctx context.Context, requestID string) ([]byte, error)
	GetResponse(ctx context.Context, requestID string) ([]byte, error)
	FindByTag(ctx context.Context, tag string) ([]string, error)
	Async() AsyncRecorder
}

// AsyncRecorder defines the asynchronous methods for the recorder.
type AsyncRecorder interface {
	RecordRequest(ctx context.Context, primaryID *string, requestID string, request []byte, tags map[string]string) <-chan error
	RecordResponse(ctx context.Context, primaryID *string, requestID string, response []byte, tags map[string]string) <-chan error
	RecordError(ctx context.Context, id *string, requestID string, err error, tags map[string]string) <-chan error
	RecordMetrics(ctx context.Context, primaryID *string, requestID string, metrics map[string]string, tags map[string]string) <-chan error
	GetRequest(ctx context.Context, requestID string) <-chan Result
	GetResponse(ctx context.Context, requestID string) <-chan Result
	FindByTag(ctx context.Context, tag string) <-chan FindByTagResult
}
