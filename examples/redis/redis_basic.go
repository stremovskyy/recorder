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
		Addr:           "localhost:6379",
		Password:       "",
		DB:             13,
		Prefix:         "myapp",
		DefaultTTL:     24 * time.Hour,
		CompressionLvl: 5,
		Debug:          true,
	}

	rec := redis_recorder.NewRedisRecorder(options)

	// Record a request
	err := rec.RecordRequest(context.Background(), nil, "req1", []byte("{\"key\": \"value\"}"), nil)
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
