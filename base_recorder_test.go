package recorder

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type stubStorage struct {
	saveFn func(context.Context, Record) error
	loadFn func(context.Context, RecordType, string) ([]byte, error)
	findFn func(context.Context, string) ([]string, error)
}

func (s stubStorage) Save(ctx context.Context, record Record) error {
	if s.saveFn != nil {
		return s.saveFn(ctx, record)
	}
	return nil
}

func (s stubStorage) Load(ctx context.Context, recordType RecordType, requestID string) ([]byte, error) {
	if s.loadFn != nil {
		return s.loadFn(ctx, recordType, requestID)
	}
	return nil, nil
}

func (s stubStorage) FindByTag(ctx context.Context, tag string) ([]string, error) {
	if s.findFn != nil {
		return s.findFn(ctx, tag)
	}
	return nil, nil
}

func TestRecordRequestDelegatesToStorage(t *testing.T) {
	ctx := context.Background()
	originalTags := map[string]string{"foo": "bar"}
	primaryID := "primary"

	var captured Record
	storage := stubStorage{
		saveFn: func(_ context.Context, record Record) error {
			captured = record
			return nil
		},
	}

	rec := New(storage)
	if err := rec.RecordRequest(ctx, &primaryID, "req-1", []byte("payload"), originalTags); err != nil {
		t.Fatalf("RecordRequest returned error: %v", err)
	}

	if captured.Type != RecordTypeRequest {
		t.Fatalf("expected record type %q, got %q", RecordTypeRequest, captured.Type)
	}
	if captured.PrimaryID == nil || *captured.PrimaryID != primaryID {
		t.Fatalf("expected primary id %q, got %+v", primaryID, captured.PrimaryID)
	}
	if captured.RequestID != "req-1" {
		t.Fatalf("expected request id req-1, got %s", captured.RequestID)
	}
	if string(captured.Payload) != "payload" {
		t.Fatalf("expected payload 'payload', got %s", string(captured.Payload))
	}

	// ensure tags are cloned (mutating the original must not affect stored copy)
	originalTags["foo"] = "mutated"
	if captured.Tags["foo"] != "bar" {
		t.Fatalf("expected cloned tags to remain 'bar', got %q", captured.Tags["foo"])
	}
}

func TestRecordResponseDelegatesToStorage(t *testing.T) {
	ctx := context.Background()
	originalTags := map[string]string{"foo": "bar"}

	var captured Record
	storage := stubStorage{
		saveFn: func(_ context.Context, record Record) error {
			captured = record
			return nil
		},
	}

	rec := New(storage)
	if err := rec.RecordResponse(ctx, nil, "req-1", []byte("payload"), originalTags); err != nil {
		t.Fatalf("RecordResponse returned error: %v", err)
	}

	if captured.Type != RecordTypeResponse {
		t.Fatalf("expected record type %q, got %q", RecordTypeResponse, captured.Type)
	}
	if captured.RequestID != "req-1" {
		t.Fatalf("expected request id req-1, got %s", captured.RequestID)
	}
	if string(captured.Payload) != "payload" {
		t.Fatalf("expected payload 'payload', got %s", string(captured.Payload))
	}
	originalTags["foo"] = "mutated"
	if captured.Tags["foo"] != "bar" {
		t.Fatalf("expected cloned tags to remain 'bar', got %q", captured.Tags["foo"])
	}
}

func TestRecordRequestValidation(t *testing.T) {
	rec := New(stubStorage{})
	if err := rec.RecordRequest(context.Background(), nil, "", []byte("payload"), nil); err == nil {
		t.Fatal("expected error for empty request id, got nil")
	}
	if err := rec.RecordRequest(context.Background(), nil, "req", nil, nil); err == nil {
		t.Fatal("expected error for empty payload, got nil")
	}
}

func TestRecordMetricsMarshalsPayload(t *testing.T) {
	ctx := context.Background()
	metrics := map[string]string{"latency": "10ms"}

	var captured Record
	storage := stubStorage{
		saveFn: func(_ context.Context, record Record) error {
			captured = record
			return nil
		},
	}

	rec := New(storage)
	if err := rec.RecordMetrics(ctx, nil, "req-2", map[string]string{"count": "1"}, metrics); err != nil {
		t.Fatalf("RecordMetrics returned error: %v", err)
	}

	if captured.Type != RecordTypeMetrics {
		t.Fatalf("unexpected record type: %s", captured.Type)
	}

	var payload map[string]string
	if err := json.Unmarshal(captured.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload["count"] != "1" {
		t.Fatalf("unexpected payload value: %+v", payload)
	}
}

func TestRecordMetricsValidation(t *testing.T) {
	rec := New(stubStorage{})
	if err := rec.RecordMetrics(context.Background(), nil, "req", map[string]string{}, nil); err == nil {
		t.Fatal("expected error for empty metrics payload")
	}
}

func TestRecordErrorValidation(t *testing.T) {
	rec := New(stubStorage{})
	if err := rec.RecordError(context.Background(), nil, "req", nil, nil); err == nil {
		t.Fatal("expected error when err is nil")
	}
}

func TestRecordRequestPropagatesStorageError(t *testing.T) {
	wantErr := errors.New("save failed")
	storage := stubStorage{
		saveFn: func(_ context.Context, _ Record) error {
			return wantErr
		},
	}
	rec := New(storage)
	err := rec.RecordRequest(context.Background(), nil, "req", []byte("payload"), nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}

func TestGetRequestDelegatesToStorage(t *testing.T) {
	storage := stubStorage{
		loadFn: func(_ context.Context, recordType RecordType, requestID string) ([]byte, error) {
			if recordType != RecordTypeRequest {
				t.Fatalf("expected record type %q, got %q", RecordTypeRequest, recordType)
			}
			if requestID != "req" {
				t.Fatalf("expected request id 'req', got %s", requestID)
			}
			return []byte("payload"), nil
		},
	}
	rec := New(storage)
	data, err := rec.GetRequest(context.Background(), "req")
	if err != nil {
		t.Fatalf("GetRequest returned error: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("expected payload 'payload', got %s", string(data))
	}
}

func TestGetResponseDelegatesToStorage(t *testing.T) {
	storage := stubStorage{
		loadFn: func(_ context.Context, recordType RecordType, requestID string) ([]byte, error) {
			if recordType != RecordTypeResponse {
				t.Fatalf("expected record type %q, got %q", RecordTypeResponse, recordType)
			}
			if requestID != "req" {
				t.Fatalf("expected request id 'req', got %s", requestID)
			}
			return []byte("payload"), nil
		},
	}
	rec := New(storage)
	data, err := rec.GetResponse(context.Background(), "req")
	if err != nil {
		t.Fatalf("GetResponse returned error: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("expected payload 'payload', got %s", string(data))
	}
}

func TestAsyncWrapper(t *testing.T) {
	saved := make(chan Record, 4)
	storage := stubStorage{
		saveFn: func(_ context.Context, record Record) error {
			saved <- record
			return nil
		},
		loadFn: func(_ context.Context, recordType RecordType, requestID string) ([]byte, error) {
			switch recordType {
			case RecordTypeRequest:
				if requestID == "req" {
					return []byte("async-request"), nil
				}
			case RecordTypeResponse:
				if requestID == "req" {
					return []byte("async-response"), nil
				}
			}
			return nil, nil
		},
	}

	rec := New(storage)
	async := rec.Async()
	if err := <-async.RecordRequest(context.Background(), nil, "req", []byte("payload"), nil); err != nil {
		t.Fatalf("async RecordRequest returned error: %v", err)
	}
	if err := <-async.RecordResponse(context.Background(), nil, "req", []byte("resp"), nil); err != nil {
		t.Fatalf("async RecordResponse returned error: %v", err)
	}
	if err := <-async.RecordError(context.Background(), nil, "req", errors.New("boom"), nil); err != nil {
		t.Fatalf("async RecordError returned error: %v", err)
	}
	if err := <-async.RecordMetrics(context.Background(), nil, "req", map[string]string{"count": "1"}, nil); err != nil {
		t.Fatalf("async RecordMetrics returned error: %v", err)
	}

	if len(saved) != 4 {
		t.Fatalf("expected four save calls, got %d", len(saved))
	}

	requestResult := <-async.GetRequest(context.Background(), "req")
	if requestResult.Err != nil || string(requestResult.Data) != "async-request" {
		t.Fatalf("expected async request to return payload, got %+v", requestResult)
	}

	result := <-async.GetResponse(context.Background(), "req")
	if result.Err != nil || string(result.Data) != "async-response" {
		t.Fatalf("expected async response to return payload, got %+v", result)
	}

	tagResult := <-async.FindByTag(context.Background(), "tag")
	if tagResult.Tags != nil || tagResult.Err != nil {
		t.Fatalf("expected default stub find result, got %+v", tagResult)
	}
}

func TestFindByTagValidation(t *testing.T) {
	rec := New(stubStorage{})
	if _, err := rec.FindByTag(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty tag")
	}
}

func TestCloneTags(t *testing.T) {
	if clone := cloneTags(nil); clone != nil {
		t.Fatalf("expected nil clone for nil input, got %#v", clone)
	}

	original := map[string]string{"a": "1"}
	clone := cloneTags(original)
	if &clone == &original {
		t.Fatal("expected different map reference")
	}
	original["a"] = "2"
	if clone["a"] != "1" {
		t.Fatalf("expected clone to keep original value, got %s", clone["a"])
	}
}
