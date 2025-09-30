# Recorder

A Go library for recording and retrieving requests, responses, errors, and metrics. This library provides both synchronous and asynchronous methods for recording data to Redis or file storage.

## Features

- Record and retrieve requests, responses, errors, and metrics.
- Support for both Redis and file-based storage backends.
- Asynchronous methods for non-blocking operations.
- Easy to extend with custom storage backends.

## Installation

To install the `recorder` library, use `go get`:

```sh
go get github.com/stremovskyy/recorder
```

## Usage

### Interface

The `recorder` package now splits responsibilities between the high-level `Recorder` API and the pluggable `Storage` abstraction:

```go
// Storage abstracts persistence. Implement it to target any backend (SQL, NoSQL, files, etc.).
type Storage interface {
Save(ctx context.Context, record Record) error
Load(ctx context.Context, recordType RecordType, requestID string) ([]byte, error)
FindByTag(ctx context.Context, tag string) ([]string, error)
}

// Recorder is the public interface for the recorder.
type Recorder interface {
	RecordRequest(ctx context.Context, primaryID *string, requestID string, request []byte, tags map[string]string) error
	RecordResponse(ctx context.Context, primaryID *string, requestID string, response []byte, tags map[string]string) error
	RecordError(ctx context.Context, id *string, requestID string, err error, tags map[string]string) error
	RecordMetrics(ctx context.Context, primaryID *string, requestID string, metrics map[string]string, tags map[string]string) error
	GetRequest(ctx context.Context, requestID string) ([]byte, error)
	GetResponse(ctx context.Context, requestID string) ([]byte, error)
	FindByTag(ctx context.Context, tag string) ([]string, error)
	Async() AsyncRecorder
}

// AsyncRecorder defines the asynchronous methods for the recorder.
type AsyncRecorder interface {
	RecordRequest(ctx context.Context, primaryID *string, requestID string, request []byte, tags map[string]string) <-chan error
	RecordResponse(ctx context.Context, primaryID *string, requestID string, response []byte, tags map[string]string) <-chan error
	RecordError(ctx context.Context, id *string, requestID string, err error, tags map[string]string) <-chan error
	RecordMetrics(ctx context.Context, primaryID *string, requestID string, metrics map[string]string, tags map[string]string) <-chan error
	GetRequest(ctx context.Context, requestID string) <-chan Result
	GetResponse(ctx context.Context, requestID string) <-chan Result
	FindByTag(ctx context.Context, tag string) <-chan FindByTagResult
}

// New wraps a Storage implementation and returns a fully featured Recorder.
func New(storage Storage) Recorder
```

### Sensitive Data Scrubber

Use the scrubber utilities when you need to strip secrets before persisting payloads.

```go
scrub := recorder.NewScrubber()
payload := map[string]any{
    "password": "super-secret",
    "headers": map[string][]string{"Authorization": []string{"Bearer token"}},
}
masked := scrub.Scrub(payload).(map[string]any)
// masked["password"] == "[REDACTED]"

storage := yourStorage{} // implements recorder.Storage
rec := recorder.New(storage, recorder.WithScrubber(scrub))
_ = rec.RecordRequest(ctx, nil, "req-42", []byte(`{"token":"abc"}`), nil)
```

You can tailor which fields are scrubbed and how the data is transformed by declaring rules:

```go
scrub := recorder.NewScrubber(
    recorder.WithDefaultReplacement("<hidden>"),
    recorder.WithoutDefaultRules(),
)
scrub.AddRules(
    recorder.NewRule(
        "mask-token",
        recorder.MatchPathInsensitive("credentials.token"),
        recorder.MaskString('*', 0, 4),
    ),
)

rec := recorder.New(
    storage,
    recorder.WithScrubber(scrub, recorder.ScrubberFailOnError()),
)
```

For non-JSON payloads or advanced logic, supply your own sanitizers with `recorder.WithPayloadScrubber` or `recorder.WithTagScrubber`.

### Redis Implementation
#### Usage

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/stremovskyy/recorder/redis_recorder"
)

func main() {
	options := &redis_recorder.Options{
		Addr:          "localhost:6379",
		Password:      "",
		DB:            0,
		Prefix:        "myapp",
		DefaultTTL:    24 * time.Hour,
		CompressionLvl: 5,
		Debug:         true,
	}

	rec := redis_recorder.NewRedisRecorder(options)

	// Record a request
	err := rec.RecordRequest(context.Background(), nil, "req1", []byte("request data"), nil)
	if err != nil {
		log.Fatalf("Failed to record request: %v", err)
	}

	// Retrieve a request
	data, err := rec.GetRequest(context.Background(), "req1")
	if err != nil {
		log.Fatalf("Failed to get request: %v", err)
	}
	fmt.Println("Request data:", string(data))
}
```

### File-based Implementation

#### Usage

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/stremovskyy/recorder/file_recorder"
)

func main() {
	rec := file_recorder.NewFileRecorder("/path/to/store/files")

	// Record a request
	err := rec.RecordRequest(context.Background(), nil, "req1", []byte("request data"), nil)
	if err != nil {
		log.Fatalf("Failed to record request: %v", err)
	}

	// Retrieve a request
	data, err := rec.GetRequest(context.Background(), "req1")
	if err != nil {
		log.Fatalf("Failed to get request: %v", err)
	}
	fmt.Println("Request data:", string(data))
}
```

### GORM + MySQL Implementation

Use GORM with the MySQL driver and hand the configured *gorm.DB to the factory:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "gorm.io/driver/mysql"
    "gorm.io/gorm"

    "github.com/stremovskyy/recorder/gorm_recorder"
)

func main() {
    dsn := "user:password@tcp(localhost:3306)/recorder?parseTime=true"
    db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
    if err != nil {
        log.Fatalf("connect: %v", err)
    }

    rec, err := gorm_recorder.NewRecorder(db)
    if err != nil {
        log.Fatalf("build recorder: %v", err)
    }

    err = rec.RecordRequest(context.Background(), nil, "req1", []byte("request data"), map[string]string{"env": "dev"})
    if err != nil {
        log.Fatalf("record: %v", err)
    }

    data, err := rec.GetRequest(context.Background(), "req1")
    if err != nil {
        log.Fatalf("get: %v", err)
    }
fmt.Println("Request data:", string(data))
}
```

If you need different table names or extra fields on the persisted models, implement the small `RecordModel` and `TagModel` abstractions and provide them when constructing the recorder:

```go
type RequestRecord struct {
    gorm.Model
    Kind          string  `gorm:"column:kind;size:32;not null"`
    CorrelationID string  `gorm:"column:correlation_id;size:255;not null"`
    ReferenceID   *string `gorm:"column:ref_id"`
    Body          []byte  `gorm:"column:body;type:blob;not null"`
}

func (RequestRecord) TableName() string { return "custom_records" }

func (r *RequestRecord) GetID() uint            { return r.ID }
func (r *RequestRecord) GetType() string        { return r.Kind }
func (r *RequestRecord) SetType(v string)       { r.Kind = v }
func (r *RequestRecord) GetRequestID() string   { return r.CorrelationID }
func (r *RequestRecord) SetRequestID(v string)  { r.CorrelationID = v }
func (r *RequestRecord) SetPrimaryID(v *string) { if v == nil { r.ReferenceID = nil; return }; tmp := *v; r.ReferenceID = &tmp }
func (r *RequestRecord) SetPayload(data []byte) { r.Body = append(r.Body[:0], data...) }
func (r *RequestRecord) GetPayload() []byte     { return r.Body }

type RequestTag struct {
    ID       uint   `gorm:"primaryKey"`
    RecordID uint   `gorm:"column:record_ref;index"`
    Key      string `gorm:"column:tag_key"`
    Value    string `gorm:"column:tag_value"`
}

func (RequestTag) TableName() string { return "custom_tags" }

func (t *RequestTag) SetRecordID(id uint) { t.RecordID = id }
func (t *RequestTag) SetKey(k string)     { t.Key = k }
func (t *RequestTag) SetValue(v string)   { t.Value = v }

opts := gorm_recorder.NewOptions(func() *RequestRecord { return &RequestRecord{} }, func() *RequestTag { return &RequestTag{} }).
    WithRecordTable("custom_records").
    WithRecordColumns("id", "kind", "correlation_id").
    WithTagTable("custom_tags").
    WithTagColumns("record_ref", "tag_key", "tag_value")

rec, err := gorm_recorder.NewRecorderWithModels(db, opts)
if err != nil {
    log.Fatalf("build recorder: %v", err)
}
```

### Asynchronous Methods

Both implementations support asynchronous methods via the `Async()` method:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/stremovskyy/recorder/file_recorder"
)

func main() {
	rec := file_recorder.NewFileRecorder("/path/to/store/files").Async()

	// Record a request asynchronously
	resultChan := rec.RecordRequest(context.Background(), nil, "req1", []byte("request data"), nil)
	if err := <-resultChan; err != nil {
		log.Fatalf("Failed to record request: %v", err)
	}

	// Retrieve a request asynchronously
	dataChan := rec.GetRequest(context.Background(), "req1")
	result := <-dataChan
	if result.Err != nil {
		log.Fatalf("Failed to get request: %v", result.Err)
	}
	fmt.Println("Request data:", string(result.Data))
}
```

## Extending the Library

Implement the `Storage` interface to back the recorder with your own persistence layer (SQL databases, object storage, message queues, etc.). Once you have a `Storage`, wrap it with
`recorder.New(storage)` to obtain the high-level API.

```go
package sqlrecorder

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/stremovskyy/recorder"
)

type SQLStorage struct {
	db *sql.DB
}

func NewSQLRecorder(db *sql.DB) recorder.Recorder {
	return recorder.New(&SQLStorage{db: db})
}

func (s *SQLStorage) Save(ctx context.Context, record recorder.Record) error {
	tagsJSON, err := json.Marshal(record.Tags)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO recordings (type, request_id, primary_id, payload, tags)
         VALUES ($1, $2, $3, $4, $5)
         ON CONFLICT (type, request_id)
         DO UPDATE SET payload = EXCLUDED.payload, tags = EXCLUDED.tags`,
		record.Type,
		record.RequestID,
		optionalString(record.PrimaryID),
		record.Payload,
		tagsJSON,
	)
	return err
}

func (s *SQLStorage) Load(ctx context.Context, recordType recorder.RecordType, requestID string) ([]byte, error) {
	var payload []byte
	err := s.db.QueryRowContext(
		ctx,
		`SELECT payload FROM recordings WHERE type = $1 AND request_id = $2`,
		recordType,
		requestID,
	).Scan(&payload)
	return payload, err
}

func (s *SQLStorage) FindByTag(ctx context.Context, tag string) ([]string, error) {
	parts := strings.SplitN(tag, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("tag must be in key:value format")
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT request_id FROM recordings WHERE tags @> jsonb_build_object($1, $2)`,
		parts[0],
		parts[1],
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func optionalString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}
```

The SQL example above stores payloads as raw bytes and tags as JSON. Adapt the schema, serialization, and tag search logic to match your database of choice.

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository.
2. Create a new branch with a descriptive name.
3. Make your changes.
4. Commit your changes with clear commit messages.
5. Push to your fork and submit a pull request.

Please ensure your code adheres to the standard Go formatting and includes tests for any new functionality.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

## Contact

For any questions or suggestions, please open an issue on GitHub or contact the repository owner.

## Acknowledgments

Special thanks to all contributors who have helped improve this project.
