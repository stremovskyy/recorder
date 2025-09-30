package callback_recorder

import (
	"context"
	"fmt"

	"github.com/stremovskyy/recorder"
)

type SaveFunc func(ctx context.Context, record recorder.Record) error

type LoadFunc func(ctx context.Context, recordType recorder.RecordType, requestID string) ([]byte, error)

type FindFunc func(ctx context.Context, tag string) ([]string, error)

type Options struct {
	Save SaveFunc
	Load LoadFunc
	Find FindFunc
}

func New(opts Options, recorderOpts ...recorder.RecorderOption) recorder.Recorder {
	return recorder.New(&callbackStorage{opts: opts}, recorderOpts...)
}

type callbackStorage struct {
	opts Options
}

func (s *callbackStorage) Save(ctx context.Context, record recorder.Record) error {
	if s.opts.Save == nil {
		return fmt.Errorf("save not supported: no SaveFunc provided")
	}
	return s.opts.Save(ctx, record)
}

func (s *callbackStorage) Load(ctx context.Context, recordType recorder.RecordType, requestID string) ([]byte, error) {
	if s.opts.Load == nil {
		return nil, fmt.Errorf("load not supported: no LoadFunc provided")
	}
	return s.opts.Load(ctx, recordType, requestID)
}

func (s *callbackStorage) FindByTag(ctx context.Context, tag string) ([]string, error) {
	if s.opts.Find == nil {
		return nil, fmt.Errorf("FindByTag is not supported in callback_recorder when FindFunc is nil")
	}
	return s.opts.Find(ctx, tag)
}
