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

The `recorder` package defines the `Recorder` interface, which includes methods for recording and retrieving data:

```go
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
```

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

You can extend the `recorder` library by implementing the `Recorder` interface for other storage backends. Create a new package for your implementation and follow the patterns shown in the Redis and file-based implementations.

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
