package recorder

import (
	"context"
	"encoding/json"
	"fmt"
)

// RecordType categorizes the kind of payload being stored.
type RecordType string

const (
	RecordTypeRequest  RecordType = "request"
	RecordTypeResponse RecordType = "response"
	RecordTypeError    RecordType = "error"
	RecordTypeMetrics  RecordType = "metrics"
)

// Record represents a single item to persist in a storage backend.
type Record struct {
	Type      RecordType
	PrimaryID *string
	RequestID string
	Payload   []byte
	Tags      map[string]string
}

// Storage abstracts the persistence layer used by Recorder implementations.
type Storage interface {
	Save(ctx context.Context, record Record) error
	Load(ctx context.Context, recordType RecordType, requestID string) ([]byte, error)
	FindByTag(ctx context.Context, tag string) ([]string, error)
}

// New constructs a Recorder backed by the provided Storage implementation.
func New(storage Storage, opts ...RecorderOption) Recorder {
	if storage == nil {
		panic("recorder: storage must not be nil")
	}

	var cfg recorderOptions
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}

	return &baseRecorder{
		storage:         storage,
		payloadScrubber: cfg.payloadScrubber,
		tagScrubber:     cfg.tagScrubber,
	}
}

type baseRecorder struct {
	storage         Storage
	payloadScrubber PayloadScrubFunc
	tagScrubber     TagScrubFunc
}

func (r *baseRecorder) RecordRequest(ctx context.Context, primaryID *string, requestID string, request []byte, tags map[string]string) error {
	if requestID == "" {
		return fmt.Errorf("requestID cannot be empty")
	}
	if len(request) == 0 {
		return fmt.Errorf("request cannot be nil or empty")
	}

	sanitizedPayload, err := r.scrubPayload(RecordTypeRequest, request)
	if err != nil {
		return fmt.Errorf("scrub request payload: %w", err)
	}
	sanitizedTags, err := r.prepareTags(RecordTypeRequest, tags)
	if err != nil {
		return fmt.Errorf("scrub request tags: %w", err)
	}

	return r.storage.Save(ctx, Record{
		Type:      RecordTypeRequest,
		PrimaryID: primaryID,
		RequestID: requestID,
		Payload:   sanitizedPayload,
		Tags:      sanitizedTags,
	})
}

func (r *baseRecorder) RecordResponse(ctx context.Context, primaryID *string, requestID string, response []byte, tags map[string]string) error {
	if requestID == "" {
		return fmt.Errorf("requestID cannot be empty")
	}
	if len(response) == 0 {
		return fmt.Errorf("response cannot be nil or empty")
	}

	sanitizedPayload, err := r.scrubPayload(RecordTypeResponse, response)
	if err != nil {
		return fmt.Errorf("scrub response payload: %w", err)
	}
	sanitizedTags, err := r.prepareTags(RecordTypeResponse, tags)
	if err != nil {
		return fmt.Errorf("scrub response tags: %w", err)
	}

	return r.storage.Save(ctx, Record{
		Type:      RecordTypeResponse,
		PrimaryID: primaryID,
		RequestID: requestID,
		Payload:   sanitizedPayload,
		Tags:      sanitizedTags,
	})
}

func (r *baseRecorder) RecordError(ctx context.Context, id *string, requestID string, err error, tags map[string]string) error {
	if err == nil {
		return fmt.Errorf("error cannot be nil")
	}
	if requestID == "" {
		return fmt.Errorf("requestID cannot be empty")
	}

	sanitizedPayload, scrubErr := r.scrubPayload(RecordTypeError, []byte(err.Error()))
	if scrubErr != nil {
		return fmt.Errorf("scrub error payload: %w", scrubErr)
	}
	sanitizedTags, scrubErr := r.prepareTags(RecordTypeError, tags)
	if scrubErr != nil {
		return fmt.Errorf("scrub error tags: %w", scrubErr)
	}

	return r.storage.Save(ctx, Record{
		Type:      RecordTypeError,
		PrimaryID: id,
		RequestID: requestID,
		Payload:   sanitizedPayload,
		Tags:      sanitizedTags,
	})
}

func (r *baseRecorder) RecordMetrics(ctx context.Context, primaryID *string, requestID string, metrics map[string]string, tags map[string]string) error {
	if requestID == "" {
		return fmt.Errorf("requestID cannot be empty")
	}
	if len(metrics) == 0 {
		return fmt.Errorf("metrics cannot be nil or empty")
	}

	jsonData, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("cannot marshal metrics: %w", err)
	}
	sanitizedPayload, scrubErr := r.scrubPayload(RecordTypeMetrics, jsonData)
	if scrubErr != nil {
		return fmt.Errorf("scrub metrics payload: %w", scrubErr)
	}
	sanitizedTags, scrubErr := r.prepareTags(RecordTypeMetrics, tags)
	if scrubErr != nil {
		return fmt.Errorf("scrub metrics tags: %w", scrubErr)
	}

	return r.storage.Save(ctx, Record{
		Type:      RecordTypeMetrics,
		PrimaryID: primaryID,
		RequestID: requestID,
		Payload:   sanitizedPayload,
		Tags:      sanitizedTags,
	})
}

func (r *baseRecorder) GetRequest(ctx context.Context, requestID string) ([]byte, error) {
	if requestID == "" {
		return nil, fmt.Errorf("requestID cannot be empty")
	}
	return r.storage.Load(ctx, RecordTypeRequest, requestID)
}

func (r *baseRecorder) GetResponse(ctx context.Context, requestID string) ([]byte, error) {
	if requestID == "" {
		return nil, fmt.Errorf("requestID cannot be empty")
	}
	return r.storage.Load(ctx, RecordTypeResponse, requestID)
}

func (r *baseRecorder) FindByTag(ctx context.Context, tag string) ([]string, error) {
	if tag == "" {
		return nil, fmt.Errorf("tag cannot be empty")
	}
	return r.storage.FindByTag(ctx, tag)
}

func (r *baseRecorder) Async() AsyncRecorder {
	return &asyncRecorder{base: r}
}

type asyncRecorder struct {
	base *baseRecorder
}

func (ar *asyncRecorder) RecordRequest(ctx context.Context, primaryID *string, requestID string, request []byte, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.base.RecordRequest(ctx, primaryID, requestID, request, tags)
	}()
	return result
}

func (ar *asyncRecorder) RecordResponse(ctx context.Context, primaryID *string, requestID string, response []byte, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.base.RecordResponse(ctx, primaryID, requestID, response, tags)
	}()
	return result
}

func (ar *asyncRecorder) RecordError(ctx context.Context, id *string, requestID string, err error, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.base.RecordError(ctx, id, requestID, err, tags)
	}()
	return result
}

func (ar *asyncRecorder) RecordMetrics(ctx context.Context, primaryID *string, requestID string, metrics map[string]string, tags map[string]string) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- ar.base.RecordMetrics(ctx, primaryID, requestID, metrics, tags)
	}()
	return result
}

func (ar *asyncRecorder) GetRequest(ctx context.Context, requestID string) <-chan Result {
	resultChan := make(chan Result, 1)
	go func() {
		data, err := ar.base.GetRequest(ctx, requestID)
		resultChan <- Result{Data: data, Err: err}
	}()
	return resultChan
}

func (ar *asyncRecorder) GetResponse(ctx context.Context, requestID string) <-chan Result {
	resultChan := make(chan Result, 1)
	go func() {
		data, err := ar.base.GetResponse(ctx, requestID)
		resultChan <- Result{Data: data, Err: err}
	}()
	return resultChan
}

func (ar *asyncRecorder) FindByTag(ctx context.Context, tag string) <-chan FindByTagResult {
	resultChan := make(chan FindByTagResult, 1)
	go func() {
		tags, err := ar.base.FindByTag(ctx, tag)
		resultChan <- FindByTagResult{Tags: tags, Err: err}
	}()
	return resultChan
}

func (r *baseRecorder) scrubPayload(recordType RecordType, payload []byte) ([]byte, error) {
	if r.payloadScrubber == nil || len(payload) == 0 {
		return payload, nil
	}
	return r.payloadScrubber(recordType, append([]byte(nil), payload...))
}

func (r *baseRecorder) prepareTags(recordType RecordType, tags map[string]string) (map[string]string, error) {
	cloned := cloneTags(tags)
	if r.tagScrubber == nil || len(cloned) == 0 {
		return cloned, nil
	}
	return r.tagScrubber(recordType, cloned)
}

func cloneTags(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(tags))
	for k, v := range tags {
		cloned[k] = v
	}
	return cloned
}
