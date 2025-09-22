package gorm_recorder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/stremovskyy/recorder"
)

type gormStorage[R RecordModel, T TagModel] struct {
	db   *gorm.DB
	opts modelOptions[R, T]
}

// NewRecorder constructs a recorder backed by GORM using the default models provided by the package.
func NewRecorder(db *gorm.DB) (recorder.Recorder, error) {
	defaultOpts := NewOptions(func() *recordModel { return &recordModel{} }, func() *recordTag { return &recordTag{} })
	prepared, err := defaultOpts.prepare()
	if err != nil {
		return nil, err
	}
	return newRecorderWithPreparedOptions[*recordModel, *recordTag](db, prepared)
}

// NewRecorderWithModels constructs a recorder backed by GORM using the supplied model abstraction.
func NewRecorderWithModels[R RecordModel, T TagModel](db *gorm.DB, opts modelOptions[R, T]) (recorder.Recorder, error) {
	prepared, err := opts.prepare()
	if err != nil {
		return nil, err
	}
	return newRecorderWithPreparedOptions[R, T](db, prepared)
}

func newRecorderWithPreparedOptions[R RecordModel, T TagModel](db *gorm.DB, opts modelOptions[R, T]) (recorder.Recorder, error) {
	if db == nil {
		return nil, fmt.Errorf("gorm recorder: db must not be nil")
	}

	if err := db.AutoMigrate(opts.recordFactory(), opts.tagFactory()); err != nil {
		return nil, fmt.Errorf("gorm recorder: auto migrate failed: %w", err)
	}

	storage := &gormStorage[R, T]{
		db:   db,
		opts: opts,
	}
	return recorder.New(storage), nil
}

func (s *gormStorage[R, T]) Save(ctx context.Context, record recorder.Record) error {
	if len(record.Payload) == 0 {
		return fmt.Errorf("gorm recorder: payload cannot be empty")
	}

	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}

	model := s.opts.recordFactory()
	err := tx.Where(map[string]any{
		s.opts.recordTypeColumn:      string(record.Type),
		s.opts.recordRequestIDColumn: record.RequestID,
	}).First(model).Error

	switch {
	case err == nil:
		// existing record loaded
	case errors.Is(err, gorm.ErrRecordNotFound):
		model.SetType(string(record.Type))
		model.SetRequestID(record.RequestID)
	case err != nil:
		tx.Rollback()
		return err
	}

	model.SetPrimaryID(record.PrimaryID)
	model.SetPayload(record.Payload)

	if err := tx.Save(model).Error; err != nil {
		tx.Rollback()
		return err
	}

	recordID := model.GetID()

	tagModel := s.opts.tagFactory()
	if err := tx.Where(map[string]any{s.opts.tagRecordIDColumn: recordID}).Delete(tagModel).Error; err != nil {
		tx.Rollback()
		return err
	}

	if len(record.Tags) > 0 {
		tags := make([]T, 0, len(record.Tags))
		for key, value := range record.Tags {
			tag := s.opts.tagFactory()
			tag.SetRecordID(recordID)
			tag.SetKey(key)
			tag.SetValue(value)
			tags = append(tags, tag)
		}
		if err := tx.Create(&tags).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

func (s *gormStorage[R, T]) Load(ctx context.Context, recordType recorder.RecordType, requestID string) ([]byte, error) {
	model := s.opts.recordFactory()
	err := s.db.WithContext(ctx).
		Where(map[string]any{
			s.opts.recordTypeColumn:      string(recordType),
			s.opts.recordRequestIDColumn: requestID,
		}).
		First(model).Error
	if err != nil {
		return nil, err
	}
	payload := model.GetPayload()
	if len(payload) == 0 {
		return nil, fmt.Errorf("gorm recorder: empty payload for record %s/%s", recordType, requestID)
	}
	result := make([]byte, len(payload))
	copy(result, payload)
	return result, nil
}

func (s *gormStorage[R, T]) FindByTag(ctx context.Context, tag string) ([]string, error) {
	key, value, err := splitTag(tag)
	if err != nil {
		return nil, err
	}

	selectClause := fmt.Sprintf("%s.%s AS record_type, %s.%s AS request_id",
		s.opts.recordTable, s.opts.recordTypeColumn,
		s.opts.recordTable, s.opts.recordRequestIDColumn,
	)
	joinClause := fmt.Sprintf("JOIN %s ON %s.%s = %s.%s",
		s.opts.recordTable,
		s.opts.recordTable, s.opts.recordIDColumn,
		s.opts.tagTable, s.opts.tagRecordIDColumn,
	)
	whereClause := fmt.Sprintf("%s.%s = ? AND %s.%s = ?",
		s.opts.tagTable, s.opts.tagKeyColumn,
		s.opts.tagTable, s.opts.tagValueColumn,
	)

	type row struct {
		RecordType string `gorm:"column:record_type"`
		RequestID  string `gorm:"column:request_id"`
	}

	var rows []row
	err = s.db.WithContext(ctx).
		Table(s.opts.tagTable).
		Select(selectClause).
		Joins(joinClause).
		Where(whereClause, key, value).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	results := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		composite := fmt.Sprintf("%s:%s", r.RecordType, r.RequestID)
		if _, ok := seen[composite]; ok {
			continue
		}
		seen[composite] = struct{}{}
		results = append(results, composite)
	}

	return results, nil
}

// default GORM models implementing the RecordModel and TagModel abstractions.
type recordModel struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Type      string      `gorm:"size:32;not null;index:idx_type_request,priority:1"`
	RequestID string      `gorm:"size:255;not null;index:idx_type_request,priority:2"`
	PrimaryID *string     `gorm:"size:255"`
	Payload   []byte      `gorm:"type:longblob;not null"`
	Tags      []recordTag `gorm:"constraint:OnDelete:CASCADE;foreignKey:RecordID"`
}

type recordTag struct {
	ID       uint   `gorm:"primaryKey"`
	RecordID uint   `gorm:"index;not null"`
	Key      string `gorm:"size:128;not null;index:idx_tag_key_value,priority:1"`
	Value    string `gorm:"size:512;not null;index:idx_tag_key_value,priority:2"`
}

func (recordModel) TableName() string {
	return "recorder_records"
}

func (recordTag) TableName() string {
	return "recorder_tags"
}

func (m *recordModel) GetID() uint {
	return m.ID
}

func (m *recordModel) GetType() string {
	return m.Type
}

func (m *recordModel) SetType(value string) {
	m.Type = value
}

func (m *recordModel) GetRequestID() string {
	return m.RequestID
}

func (m *recordModel) SetRequestID(value string) {
	m.RequestID = value
}

func (m *recordModel) SetPrimaryID(value *string) {
	if value == nil {
		m.PrimaryID = nil
		return
	}
	v := *value
	m.PrimaryID = &v
}

func (m *recordModel) SetPayload(payload []byte) {
	if payload == nil {
		m.Payload = nil
		return
	}
	m.Payload = append(m.Payload[:0], payload...)
}

func (m *recordModel) GetPayload() []byte {
	return m.Payload
}

func (t *recordTag) SetRecordID(id uint) {
	t.RecordID = id
}

func (t *recordTag) SetKey(key string) {
	t.Key = key
}

func (t *recordTag) SetValue(value string) {
	t.Value = value
}

func splitTag(tag string) (string, string, error) {
	if tag == "" {
		return "", "", fmt.Errorf("gorm recorder: tag cannot be empty")
	}
	parts := strings.SplitN(tag, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("gorm recorder: tag must be in key:value format")
	}
	return parts[0], parts[1], nil
}
