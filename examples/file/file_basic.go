package main

import (
	"context"
	"fmt"
	"log"

	"github.com/stremovskyy/recorder/file_recorder"
)

func main() {
	rec := file_recorder.NewFileRecorder("./recorder")

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
