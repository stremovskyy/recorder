package gorm_recorder

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stremovskyy/recorder"
)

func newTestRecorder(t *testing.T) recorder.Recorder {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}

	rec, err := NewRecorder(db)
	if err != nil {
		t.Fatalf("failed to create gorm recorder: %v", err)
	}
	return rec
}

func TestGORMRecorderRoundTrip(t *testing.T) {
	rec := newTestRecorder(t)
	ctx := context.Background()

	tags := map[string]string{"env": "dev", "team": "core"}
	if err := rec.RecordRequest(ctx, nil, "req1", []byte("request"), tags); err != nil {
		t.Fatalf("RecordRequest returned error: %v", err)
	}

	// second call should update existing record and replace tags
	if err := rec.RecordRequest(ctx, nil, "req1", []byte("request-updated"), map[string]string{"env": "prod"}); err != nil {
		t.Fatalf("RecordRequest update returned error: %v", err)
	}

	payload, err := rec.GetRequest(ctx, "req1")
	if err != nil {
		t.Fatalf("GetRequest returned error: %v", err)
	}
	if string(payload) != "request-updated" {
		t.Fatalf("expected updated payload, got %s", string(payload))
	}

	if err := rec.RecordResponse(ctx, nil, "req1", []byte("response"), map[string]string{"env": "prod"}); err != nil {
		t.Fatalf("RecordResponse returned error: %v", err)
	}

	data, err := rec.GetResponse(ctx, "req1")
	if err != nil {
		t.Fatalf("GetResponse returned error: %v", err)
	}
	if string(data) != "response" {
		t.Fatalf("expected response payload, got %s", string(data))
	}

	ids, err := rec.FindByTag(ctx, "env:prod")
	if err != nil {
		t.Fatalf("FindByTag returned error: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("expected results for tag search")
	}
}

func TestGORMRecorderErrorHandling(t *testing.T) {
	rec := newTestRecorder(t)
	ctx := context.Background()

	if err := rec.RecordResponse(ctx, nil, "", []byte("data"), nil); err == nil {
		t.Fatal("expected validation error for empty request id")
	}

	if _, err := rec.FindByTag(ctx, "invalid-tag-format"); err == nil {
		t.Fatal("expected error for invalid tag format")
	}
}

func TestGORMRecorderWithCustomModels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}

	opts := NewOptions(func() *customRecordModel { return &customRecordModel{} }, func() *customTagModel { return &customTagModel{} }).
		WithRecordTable("custom_records").
		WithRecordColumns("id", "kind", "correlation_id").
		WithTagTable("custom_tags").
		WithTagColumns("record_ref", "t_key", "t_value")

	rec, err := NewRecorderWithModels(db, opts)
	if err != nil {
		t.Fatalf("failed to create recorder with custom models: %v", err)
	}

	ctx := context.Background()
	primary := "primary-1"
	if err := rec.RecordRequest(ctx, &primary, "req-custom", []byte("custom-request"), map[string]string{"tenant": "alpha"}); err != nil {
		t.Fatalf("RecordRequest returned error: %v", err)
	}

	if err := rec.RecordResponse(ctx, nil, "req-custom", []byte("custom-response"), map[string]string{"tenant": "alpha"}); err != nil {
		t.Fatalf("RecordResponse returned error: %v", err)
	}

	payload, err := rec.GetRequest(ctx, "req-custom")
	if err != nil {
		t.Fatalf("GetRequest returned error: %v", err)
	}
	if string(payload) != "custom-request" {
		t.Fatalf("unexpected request payload: %s", string(payload))
	}

	data, err := rec.GetResponse(ctx, "req-custom")
	if err != nil {
		t.Fatalf("GetResponse returned error: %v", err)
	}
	if string(data) != "custom-response" {
		t.Fatalf("unexpected response payload: %s", string(data))
	}

	ids, err := rec.FindByTag(ctx, "tenant:alpha")
	if err != nil {
		t.Fatalf("FindByTag returned error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected two records (request and response) for tenant alpha, got %d", len(ids))
	}
}

type customRecordModel struct {
	gorm.Model
	Kind          string  `gorm:"column:kind;size:32;not null;index:idx_kind_request,priority:1"`
	CorrelationID string  `gorm:"column:correlation_id;size:255;not null;index:idx_kind_request,priority:2"`
	ReferenceID   *string `gorm:"column:ref_id;size:255"`
	Body          []byte  `gorm:"column:body;type:blob;not null"`
}

func (customRecordModel) TableName() string {
	return "custom_records"
}

func (m *customRecordModel) GetID() uint {
	return m.ID
}

func (m *customRecordModel) GetType() string {
	return m.Kind
}

func (m *customRecordModel) SetType(value string) {
	m.Kind = value
}

func (m *customRecordModel) GetRequestID() string {
	return m.CorrelationID
}

func (m *customRecordModel) SetRequestID(value string) {
	m.CorrelationID = value
}

func (m *customRecordModel) SetPrimaryID(value *string) {
	if value == nil {
		m.ReferenceID = nil
		return
	}
	v := *value
	m.ReferenceID = &v
}

func (m *customRecordModel) SetPayload(payload []byte) {
	m.Body = append(m.Body[:0], payload...)
}

func (m *customRecordModel) GetPayload() []byte {
	return m.Body
}

type customTagModel struct {
	ID       uint   `gorm:"primaryKey"`
	RecordID uint   `gorm:"column:record_ref;index"`
	Key      string `gorm:"column:t_key;size:128;not null;index:idx_custom_tags,priority:1"`
	Value    string `gorm:"column:t_value;size:512;not null;index:idx_custom_tags,priority:2"`
}

func (customTagModel) TableName() string {
	return "custom_tags"
}

func (t *customTagModel) SetRecordID(id uint) {
	t.RecordID = id
}

func (t *customTagModel) SetKey(key string) {
	t.Key = key
}

func (t *customTagModel) SetValue(value string) {
	t.Value = value
}
