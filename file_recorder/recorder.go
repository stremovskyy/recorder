package file_recorder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/stremovskyy/recorder"
)

type fileStorage struct {
	basePath string
	mu       sync.Mutex
}

// NewFileRecorder creates a Recorder backed by local file storage.
func NewFileRecorder(basePath string, recorderOpts ...recorder.RecorderOption) recorder.Recorder {
	return recorder.New(
		&fileStorage{
			basePath: basePath,
		},
		recorderOpts...,
	)
}

func (s *fileStorage) Save(ctx context.Context, record recorder.Record) error {
	_ = ctx
	if len(record.Payload) == 0 {
		return fmt.Errorf("data cannot be nil or empty")
	}

	prefix, err := s.prefixFor(record.Type)
	if err != nil {
		return err
	}

	id := record.RequestID
	if record.PrimaryID != nil && *record.PrimaryID != "" {
		id = fmt.Sprintf("%s_%s", *record.PrimaryID, id)
	}

	return s.saveToFile(prefix, id, record.Payload)
}

func (s *fileStorage) Load(ctx context.Context, recordType recorder.RecordType, requestID string) ([]byte, error) {
	_ = ctx
	prefix, err := s.prefixFor(recordType)
	if err != nil {
		return nil, err
	}

	return s.loadFromFile(prefix, requestID)
}

func (s *fileStorage) FindByTag(ctx context.Context, tag string) ([]string, error) {
	// Since the file-based implementation doesn't use tags in the same way as Redis, this method isn't applicable.
	return nil, fmt.Errorf("FindByTag is not supported in file_recorder-based recorder")
}

func (s *fileStorage) prefixFor(recordType recorder.RecordType) (string, error) {
	switch recordType {
	case recorder.RecordTypeRequest:
		return "requests", nil
	case recorder.RecordTypeResponse:
		return "responses", nil
	case recorder.RecordTypeError:
		return "errors", nil
	case recorder.RecordTypeMetrics:
		return "metrics", nil
	default:
		return "", fmt.Errorf("unsupported record type: %s", recordType)
	}
}

func (s *fileStorage) saveToFile(prefix, id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.basePath, prefix, id+".json")
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

func (s *fileStorage) loadFromFile(prefix, id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.basePath, prefix, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	return data, nil
}
