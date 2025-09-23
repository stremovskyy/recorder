package gorm_recorder

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stremovskyy/recorder"
)

// Global counters for adaptive retry strategy
var (
	deadlockCounter int64
	successCounter  int64
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

	// Adaptive retry configuration based on deadlock frequency
	maxRetries := s.calculateAdaptiveRetries()
	baseDelay := s.calculateAdaptiveDelay()

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := s.calculateBackoffDelay(attempt, baseDelay)

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := s.saveWithOptimizedTransaction(ctx, record)
		if err == nil {
			atomic.AddInt64(&successCounter, 1)
			return nil
		}

		// Check if it's a deadlock error
		if isDeadlockError(err) {
			atomic.AddInt64(&deadlockCounter, 1)
			lastErr = err
			continue
		}

		// If it's not a deadlock, return immediately
		return err
	}

	return fmt.Errorf("gorm recorder: failed after %d retries due to deadlocks: %w", maxRetries, lastErr)
}

// calculateAdaptiveRetries adjusts retry count based on deadlock frequency
func (s *gormStorage[R, T]) calculateAdaptiveRetries() int {
	deadlocks := atomic.LoadInt64(&deadlockCounter)
	successes := atomic.LoadInt64(&successCounter)

	if successes == 0 {
		return 5 // Default when no data
	}

	deadlockRate := float64(deadlocks) / float64(deadlocks+successes)

	switch {
	case deadlockRate > 0.1: // High contention
		return 8
	case deadlockRate > 0.05: // Medium contention
		return 6
	default: // Low contention
		return 3
	}
}

// calculateAdaptiveDelay adjusts base delay based on system load
func (s *gormStorage[R, T]) calculateAdaptiveDelay() time.Duration {
	deadlocks := atomic.LoadInt64(&deadlockCounter)
	successes := atomic.LoadInt64(&successCounter)

	if successes == 0 {
		return 100 * time.Millisecond
	}

	deadlockRate := float64(deadlocks) / float64(deadlocks+successes)

	switch {
	case deadlockRate > 0.1:
		return 200 * time.Millisecond // Longer delays for high contention
	case deadlockRate > 0.05:
		return 150 * time.Millisecond
	default:
		return 50 * time.Millisecond
	}
}

// calculateBackoffDelay uses exponential backoff with crypto-random jitter
func (s *gormStorage[R, T]) calculateBackoffDelay(attempt int, baseDelay time.Duration) time.Duration {
	// Exponential backoff with cap
	multiplier := math.Pow(2, float64(attempt-1))
	if multiplier > 16 { // Cap at 16x base delay
		multiplier = 16
	}

	delay := time.Duration(float64(baseDelay) * multiplier)

	// Crypto-random jitter to avoid thundering herd
	jitterBytes := make([]byte, 8)
	if _, err := rand.Read(jitterBytes); err == nil {
		jitterValue := binary.BigEndian.Uint64(jitterBytes)
		maxJitter := int64(delay / 2)
		if maxJitter > 0 {
			jitter := time.Duration(int64(jitterValue) % maxJitter)
			delay += jitter
		}
	}

	return delay
}

// saveWithOptimizedTransaction splits the transaction to reduce lock scope
func (s *gormStorage[R, T]) saveWithOptimizedTransaction(ctx context.Context, record recorder.Record) error {
	// Step 1: Save/Update the main record first
	recordID, err := s.saveRecord(ctx, record)
	if err != nil {
		return err
	}

	// Step 2: Handle tags in a separate, shorter transaction
	if len(record.Tags) > 0 {
		return s.saveTags(ctx, recordID, record.Tags)
	}

	return nil
}

// saveRecord handles the main record in a focused transaction
func (s *gormStorage[R, T]) saveRecord(ctx context.Context, record recorder.Record) (uint, error) {
	var recordID uint

	err := s.db.WithContext(ctx).Transaction(
		func(tx *gorm.DB) error {
			model := s.opts.recordFactory()
			err := tx.Where(
				map[string]any{
					s.opts.recordTypeColumn:      string(record.Type),
					s.opts.recordRequestIDColumn: record.RequestID,
				},
			).First(model).Error

			switch {
			case err == nil:
				// existing record loaded
			case errors.Is(err, gorm.ErrRecordNotFound):
				model.SetType(string(record.Type))
				model.SetRequestID(record.RequestID)
			default:
				return err
			}

			model.SetPrimaryID(record.PrimaryID)
			model.SetPayload(record.Payload)

			if err := tx.Save(model).Error; err != nil {
				return err
			}

			recordID = model.GetID()
			return nil
		},
	)

	return recordID, err
}

// saveTags handles tag operations in optimized batches
func (s *gormStorage[R, T]) saveTags(ctx context.Context, recordID uint, tags map[string]string) error {
	// Use smaller, focused transactions for tags to reduce lock time
	return s.db.WithContext(ctx).Transaction(
		func(tx *gorm.DB) error {
			// For MySQL, reduce gap locking by using READ COMMITTED for this short-living transaction
			if tx.Dialector != nil && tx.Dialector.Name() == "mysql" {
				// Ignore error if the server/role disallows changing tx level here
				_ = tx.Exec("SET TRANSACTION ISOLATION LEVEL READ COMMITTED").Error
			}

			// Quick delete of existing tags to ensure idempotency
			tagModel := s.opts.tagFactory()
			if err := tx.Where(map[string]any{s.opts.tagRecordIDColumn: recordID}).Delete(tagModel).Error; err != nil {
				return err
			}

			if len(tags) == 0 {
				return nil
			}

			// Convert map to a deterministically ordered slice to avoid deadlocks by consistent lock order
			type kv struct{ k, v string }
			ordered := make([]kv, 0, len(tags))
			for k, v := range tags {
				ordered = append(ordered, kv{k: k, v: v})
			}
			sort.Slice(
				ordered, func(i, j int) bool {
					if ordered[i].k == ordered[j].k {
						return ordered[i].v < ordered[j].v
					}
					return ordered[i].k < ordered[j].k
				},
			)

			// Build tag models in the same order
			tagModels := make([]T, 0, len(ordered))
			for _, p := range ordered {
				tag := s.opts.tagFactory()
				tag.SetRecordID(recordID)
				tag.SetKey(p.k)
				tag.SetValue(p.v)
				tagModels = append(tagModels, tag)
			}

			// Insert in small, deterministic batches to reduce lock duration
			const optimalBatchSize = 2
			for i := 0; i < len(tagModels); i += optimalBatchSize {
				end := i + optimalBatchSize
				if end > len(tagModels) {
					end = len(tagModels)
				}
				batch := tagModels[i:end]

				qb := tx
				if tx.Dialector != nil && tx.Dialector.Name() == "mysql" {
					// Lower lock priority a bit to reduce deadlocks under contention
					qb = qb.Clauses(clause.Insert{Modifier: "LOW_PRIORITY"})
				}

				if err := qb.CreateInBatches(&batch, optimalBatchSize).Error; err != nil {
					return err
				}
			}

			return nil
		},
	)
}

// Enhanced deadlock detection with more patterns
func isDeadlockError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "error 1213") ||
		strings.Contains(errStr, "deadlock found when trying to get lock") ||
		strings.Contains(errStr, "deadlock") ||
		strings.Contains(errStr, "lock wait timeout") ||
		strings.Contains(errStr, "error 1205") // Lock wait timeout
}

func (s *gormStorage[R, T]) Load(ctx context.Context, recordType recorder.RecordType, requestID string) ([]byte, error) {
	model := s.opts.recordFactory()
	err := s.db.WithContext(ctx).
		Where(
			map[string]any{
				s.opts.recordTypeColumn:      string(recordType),
				s.opts.recordRequestIDColumn: requestID,
			},
		).
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

	selectClause := fmt.Sprintf(
		"%s.%s AS record_type, %s.%s AS request_id",
		s.opts.recordTable, s.opts.recordTypeColumn,
		s.opts.recordTable, s.opts.recordRequestIDColumn,
	)
	joinClause := fmt.Sprintf(
		"JOIN %s ON %s.%s = %s.%s",
		s.opts.recordTable,
		s.opts.recordTable, s.opts.recordIDColumn,
		s.opts.tagTable, s.opts.tagRecordIDColumn,
	)
	whereClause := fmt.Sprintf(
		"%s.%s = ? AND %s.%s = ?",
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
