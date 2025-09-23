package callback_recorder

import (
	"context"
	"errors"
	"testing"

	"github.com/stremovskyy/recorder"
)

func TestCallbackRecorder_SaveDelegation(t *testing.T) {
	ctx := context.Background()
	primary := "p1"
	var captured recorder.Record

	rec := New(
		Options{
			Save: func(_ context.Context, r recorder.Record) error {
				captured = r
				return nil
			},
		},
	)

	tags := map[string]string{"env": "dev"}
	if err := rec.RecordRequest(ctx, &primary, "req-1", []byte("data"), tags); err != nil {
		t.Fatalf("RecordRequest error: %v", err)
	}

	if captured.Type != recorder.RecordTypeRequest {
		t.Fatalf("unexpected type: %s", captured.Type)
	}
	if captured.PrimaryID == nil || *captured.PrimaryID != primary {
		t.Fatalf("unexpected primary id: %+v", captured.PrimaryID)
	}
	if captured.RequestID != "req-1" {
		t.Fatalf("unexpected request id: %s", captured.RequestID)
	}
	if string(captured.Payload) != "data" {
		t.Fatalf("unexpected payload: %s", string(captured.Payload))
	}
	if captured.Tags["env"] != "dev" {
		t.Fatalf("unexpected tags: %+v", captured.Tags)
	}
}

func TestCallbackRecorder_LoadDelegation(t *testing.T) {
	rec := New(
		Options{
			Save: func(ctx context.Context, r recorder.Record) error { return nil },
			Load: func(_ context.Context, rt recorder.RecordType, id string) ([]byte, error) {
				if rt != recorder.RecordTypeResponse || id != "req-2" {
					return nil, errors.New("unexpected args")
				}
				return []byte("response"), nil
			},
		},
	)

	data, err := rec.GetResponse(context.Background(), "req-2")
	if err != nil {
		t.Fatalf("GetResponse error: %v", err)
	}
	if string(data) != "response" {
		t.Fatalf("unexpected data: %s", string(data))
	}
}

func TestCallbackRecorder_FindDelegation(t *testing.T) {
	returned := []string{"k1", "k2"}
	rec := New(
		Options{
			Save: func(ctx context.Context, r recorder.Record) error { return nil },
			Find: func(_ context.Context, tag string) ([]string, error) {
				if tag != "env:dev" {
					return nil, errors.New("unexpected tag")
				}
				return returned, nil
			},
		},
	)

	res, err := rec.FindByTag(context.Background(), "env:dev")
	if err != nil {
		t.Fatalf("FindByTag error: %v", err)
	}
	if len(res) != 2 || res[0] != "k1" || res[1] != "k2" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestCallbackRecorder_MissingCallbacks(t *testing.T) {
	rec := New(Options{})
	if err := rec.RecordRequest(context.Background(), nil, "req", []byte("x"), nil); err == nil {
		t.Fatal("expected error when SaveFunc is nil")
	}
	if _, err := rec.GetRequest(context.Background(), "req"); err == nil {
		t.Fatal("expected error when LoadFunc is nil")
	}
	if _, err := rec.FindByTag(context.Background(), "tag:value"); err == nil {
		t.Fatal("expected error when FindFunc is nil")
	}
}
