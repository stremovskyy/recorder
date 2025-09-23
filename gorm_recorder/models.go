package gorm_recorder

import "fmt"

// RecordModel abstracts the database model used to persist records.
// Implementations may add additional fields or gorm annotations as needed, but must
// expose the core behavior expected by the recorder via these methods.
type RecordModel interface {
	GetID() uint
	GetType() string
	SetType(value string)
	GetRequestID() string
	SetRequestID(value string)
	SetPrimaryID(value *string)
	SetPayload(payload []byte)
	GetPayload() []byte
}

// TagModel abstracts the database model used to persist tags associated with records.
type TagModel interface {
	SetRecordID(id uint)
	SetKey(key string)
	SetValue(value string)
}

// modelOptions configures how the storage interacts with the underlying GORM models.
type modelOptions[R RecordModel, T TagModel] struct {
	recordFactory func() R
	tagFactory    func() T

	recordTable           string
	recordIDColumn        string
	recordTypeColumn      string
	recordRequestIDColumn string
	tagTable              string
	tagRecordIDColumn     string
	tagKeyColumn          string
	tagValueColumn        string
}

func (o *modelOptions[R, T]) clone() modelOptions[R, T] {
	return modelOptions[R, T]{
		recordFactory:         o.recordFactory,
		tagFactory:            o.tagFactory,
		recordTable:           o.recordTable,
		recordIDColumn:        o.recordIDColumn,
		recordTypeColumn:      o.recordTypeColumn,
		recordRequestIDColumn: o.recordRequestIDColumn,
		tagTable:              o.tagTable,
		tagRecordIDColumn:     o.tagRecordIDColumn,
		tagKeyColumn:          o.tagKeyColumn,
		tagValueColumn:        o.tagValueColumn,
	}
}

// NewOptions constructs model options using the supplied factories and configuration.
func NewOptions[R RecordModel, T TagModel](recordFactory func() R, tagFactory func() T) modelOptions[R, T] {
	return modelOptions[R, T]{
		recordFactory:         recordFactory,
		tagFactory:            tagFactory,
		recordTable:           "recorder_records",
		recordIDColumn:        "id",
		recordTypeColumn:      "type",
		recordRequestIDColumn: "request_id",
		tagTable:              "recorder_tags",
		tagRecordIDColumn:     "record_id",
		tagKeyColumn:          "key",
		tagValueColumn:        "value",
	}
}

// WithRecordTable overrides the table name used for records.
func (o modelOptions[R, T]) WithRecordTable(table string) modelOptions[R, T] {
	o.recordTable = table
	return o
}

// WithRecordColumns overrides the column names used for the core record fields.
func (o modelOptions[R, T]) WithRecordColumns(id, recordType, requestID string) modelOptions[R, T] {
	o.recordIDColumn = id
	o.recordTypeColumn = recordType
	o.recordRequestIDColumn = requestID
	return o
}

// WithTagTable overrides the table name used for tags.
func (o modelOptions[R, T]) WithTagTable(table string) modelOptions[R, T] {
	o.tagTable = table
	return o
}

// WithTagColumns overrides the column names used for the core tag fields.
func (o modelOptions[R, T]) WithTagColumns(recordID, key, value string) modelOptions[R, T] {
	o.tagRecordIDColumn = recordID
	o.tagKeyColumn = key
	o.tagValueColumn = value
	return o
}

func (o modelOptions[R, T]) prepare() (modelOptions[R, T], error) {
	if o.recordFactory == nil {
		return modelOptions[R, T]{}, fmt.Errorf("gorm recorder: record factory must not be nil")
	}
	if o.tagFactory == nil {
		return modelOptions[R, T]{}, fmt.Errorf("gorm recorder: tag factory must not be nil")
	}

	prepared := o.clone()

	if prepared.recordTable == "" {
		if namer, ok := any(prepared.recordFactory()).(interface{ TableName() string }); ok {
			prepared.recordTable = namer.TableName()
		}
	}
	if prepared.recordTable == "" {
		return modelOptions[R, T]{}, fmt.Errorf("gorm recorder: record table name must not be empty")
	}

	if prepared.recordIDColumn == "" {
		prepared.recordIDColumn = "id"
	}
	if prepared.recordTypeColumn == "" {
		prepared.recordTypeColumn = "type"
	}
	if prepared.recordRequestIDColumn == "" {
		prepared.recordRequestIDColumn = "request_id"
	}

	if prepared.tagTable == "" {
		if namer, ok := any(prepared.tagFactory()).(interface{ TableName() string }); ok {
			prepared.tagTable = namer.TableName()
		}
	}
	if prepared.tagTable == "" {
		return modelOptions[R, T]{}, fmt.Errorf("gorm recorder: tag table name must not be empty")
	}

	if prepared.tagRecordIDColumn == "" {
		prepared.tagRecordIDColumn = "record_id"
	}
	// Default to actual column name used by default tag model
	if prepared.tagKeyColumn == "" {
		prepared.tagKeyColumn = "key"
	}
	if prepared.tagValueColumn == "" {
		prepared.tagValueColumn = "value"
	}

	return prepared, nil
}
