package file_recorder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stremovskyy/recorder"
)

func TestFileRecorderRoundTrip(t *testing.T) {
	dir := t.TempDir()
	rec := NewFileRecorder(dir)

	ctx := context.Background()
	if err := rec.RecordRequest(ctx, nil, "req1", []byte("hello"), nil); err != nil {
		t.Fatalf("RecordRequest returned error: %v", err)
	}

	data, err := rec.GetRequest(ctx, "req1")
	if err != nil {
		t.Fatalf("GetRequest returned error: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected payload 'hello', got %s", string(data))
	}

	path := filepath.Join(dir, "requests", "req1.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
}

func TestFileRecorderPrimaryIDPrefix(t *testing.T) {
	dir := t.TempDir()
	rec := NewFileRecorder(dir)

	primary := "user42"
	if err := rec.RecordResponse(context.Background(), &primary, "req2", []byte("resp"), nil); err != nil {
		t.Fatalf("RecordResponse returned error: %v", err)
	}

	path := filepath.Join(dir, "responses", "user42_req2.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected prefixed file at %s: %v", path, err)
	}
}

func TestFileStorageFindByTagUnsupported(t *testing.T) {
	dir := t.TempDir()
	rec := NewFileRecorder(dir)
	if _, err := rec.FindByTag(context.Background(), "any"); err == nil {
		t.Fatal("expected error for unsupported FindByTag")
	}
}

func TestFileStorageErrorPaths(t *testing.T) {
	dir := t.TempDir()
	storage := &fileStorage{basePath: dir}

	if err := storage.Save(context.Background(), recorder.Record{Type: recorder.RecordTypeRequest, RequestID: "req", Payload: nil}); err == nil {
		t.Fatal("expected error for empty payload")
	}

	if _, err := storage.prefixFor(recorder.RecordType("unknown")); err == nil {
		t.Fatal("expected error for unknown record type")
	}

	if _, err := storage.Load(context.Background(), recorder.RecordTypeRequest, "missing"); err == nil {
		t.Fatal("expected error when loading missing file")
	}
}
