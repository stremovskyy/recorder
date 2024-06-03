package file_recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/stremovskyy/recorder"
)

type fileRecorder struct {
	basePath string
	mu       sync.Mutex
}

type asyncFileRecorder struct {
	*fileRecorder
}

// NewFileRecorder creates a new instance of fileRecorder.
func NewFileRecorder(basePath string) recorder.Recorder {
	return &fileRecorder{
		basePath: basePath,
	}
}

func (r *fileRecorder) saveToFile(ctx context.Context, prefix, id string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	path := filepath.Join(r.basePath, prefix, id+".json")
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	return ioutil.WriteFile(path, data, 0644)
}

func (r *fileRecorder) loadFromFile(ctx context.Context, prefix, id string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	path := filepath.Join(r.basePath, prefix, id+".json")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	return data, nil
}

func (r *fileRecorder) record(ctx context.Context, prefix string, primaryID *string, id string, data []byte) error {
	if data == nil || len(data) == 0 {
		return fmt.Errorf("data cannot be nil or empty")
	}

	if primaryID != nil {
		id = fmt.Sprintf("%s_%s", *primaryID, id)
	}

	return r.saveToFile(ctx, prefix, id, data)
}

func (r *fileRecorder) RecordRequest(ctx context.Context, primaryID *string, requestID string, request []byte, tags map[string]string) error {
	return r.record(ctx, "requests", primaryID, requestID, request)
}

func (r *fileRecorder) RecordResponse(ctx context.Context, primaryID *string, requestID string, response []byte, tags map[string]string) error {
	return r.record(ctx, "responses", primaryID, requestID, response)
}

func (r *fileRecorder) RecordError(ctx context.Context, id *string, requestID string, err error, tags map[string]string) error {
	if err == nil {
		return fmt.Errorf("error cannot be nil")
	}
	return r.record(ctx, "errors", id, requestID, []byte(err.Error()))
}

func (r *fileRecorder) RecordMetrics(ctx context.Context, primaryID *string, requestID string, metrics map[string]string, tags map[string]string) error {
	if metrics == nil || len(metrics) == 0 {
		return fmt.Errorf("metrics cannot be nil or empty")
	}

	jsonData, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("cannot marshal metrics: %w", err)
	}
	return r.record(ctx, "metrics", primaryID, requestID, jsonData)
}

func (r *fileRecorder) GetRequest(ctx context.Context, requestID string) ([]byte, error) {
	return r.loadFromFile(ctx, "requests", requestID)
}

func (r *fileRecorder) GetResponse(ctx context.Context, requestID string) ([]byte, error) {
	return r.loadFromFile(ctx, "responses", requestID)
}

func (r *fileRecorder) FindByTag(ctx context.Context, tag string) ([]string, error) {
	// Since the file-based implementation doesn't use tags in the same way as Redis, this method isn't applicable.
	return nil, fmt.Errorf("FindByTag is not supported in file_recorder-based recorder")
}

func (r *fileRecorder) Async() recorder.AsyncRecorder {
	return &asyncFileRecorder{r}
}

// Asynchronous Methods
func (ar *asyncFileRecorder) RecordRequest(ctx context.Context, primaryID *string, requestID string, request []byte, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.fileRecorder.RecordRequest(ctx, primaryID, requestID, request, tags)
	}()
	return result
}

func (ar *asyncFileRecorder) RecordResponse(ctx context.Context, primaryID *string, requestID string, response []byte, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.fileRecorder.RecordResponse(ctx, primaryID, requestID, response, tags)
	}()
	return result
}

func (ar *asyncFileRecorder) RecordError(ctx context.Context, id *string, requestID string, err error, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.fileRecorder.RecordError(ctx, id, requestID, err, tags)
	}()
	return result
}

func (ar *asyncFileRecorder) RecordMetrics(ctx context.Context, primaryID *string, requestID string, metrics map[string]string, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.fileRecorder.RecordMetrics(ctx, primaryID, requestID, metrics, tags)
	}()
	return result
}

func (ar *asyncFileRecorder) GetRequest(ctx context.Context, requestID string) <-chan recorder.Result {
	resultChan := make(chan recorder.Result, 1)
	go func() {
		data, err := ar.fileRecorder.GetRequest(ctx, requestID)
		resultChan <- recorder.Result{Data: data, Err: err}
	}()
	return resultChan
}

func (ar *asyncFileRecorder) GetResponse(ctx context.Context, requestID string) <-chan recorder.Result {
	resultChan := make(chan recorder.Result, 1)
	go func() {
		data, err := ar.fileRecorder.GetResponse(ctx, requestID)
		resultChan <- recorder.Result{Data: data, Err: err}
	}()
	return resultChan
}

func (ar *asyncFileRecorder) FindByTag(ctx context.Context, tag string) <-chan recorder.FindByTagResult {
	resultChan := make(chan recorder.FindByTagResult, 1)
	go func() {
		tags, err := ar.fileRecorder.FindByTag(ctx, tag)
		resultChan <- recorder.FindByTagResult{Tags: tags, Err: err}
	}()
	return resultChan
}
