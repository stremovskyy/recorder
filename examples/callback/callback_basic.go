package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/stremovskyy/recorder"
	"github.com/stremovskyy/recorder/callback_recorder"
)

type memStore struct {
	mu    sync.RWMutex
	data  map[recorder.RecordType]map[string][]byte
	index map[string]map[string]struct{}
}

func newMemStore() *memStore {
	return &memStore{
		data:  make(map[recorder.RecordType]map[string][]byte),
		index: make(map[string]map[string]struct{}),
	}
}

func (m *memStore) Save(_ context.Context, r recorder.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.data[r.Type]; !ok {
		m.data[r.Type] = make(map[string][]byte)
	}

	buf := make([]byte, len(r.Payload))
	copy(buf, r.Payload)
	m.data[r.Type][r.RequestID] = buf

	if len(r.Tags) > 0 {
		composite := fmt.Sprintf("%s:%s", r.Type, r.RequestID)
		for k, v := range r.Tags {
			key := fmt.Sprintf("%s:%s", k, v)
			set, ok := m.index[key]
			if !ok {
				set = make(map[string]struct{})
				m.index[key] = set
			}
			set[composite] = struct{}{}
		}
	}
	return nil
}

func (m *memStore) Load(_ context.Context, t recorder.RecordType, requestID string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.data[t] == nil {
		return nil, fmt.Errorf("not found: %s/%s", t, requestID)
	}
	b, ok := m.data[t][requestID]
	if !ok {
		return nil, fmt.Errorf("not found: %s/%s", t, requestID)
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

func (m *memStore) Find(_ context.Context, tag string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	set := m.index[tag]
	if len(set) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

func main() {
	store := newMemStore()
	rec := callback_recorder.New(
		callback_recorder.Options{
			Save: store.Save,
			Load: store.Load,
			Find: store.Find,
		},
	)

	ctx := context.Background()

	// Record request and response with tags
	tags := map[string]string{"env": "dev", "service": "orders"}
	if err := rec.RecordRequest(ctx, nil, "req-42", []byte("request-body"), tags); err != nil {
		log.Fatalf("RecordRequest failed: %v", err)
	}
	if err := rec.RecordResponse(ctx, nil, "req-42", []byte("response-body"), tags); err != nil {
		log.Fatalf("RecordResponse failed: %v", err)
	}

	// Load back
	req, err := rec.GetRequest(ctx, "req-42")
	if err != nil {
		log.Fatalf("GetRequest failed: %v", err)
	}
	resp, err := rec.GetResponse(ctx, "req-42")
	if err != nil {
		log.Fatalf("GetResponse failed: %v", err)
	}
	fmt.Printf("request: %s\n", string(req))
	fmt.Printf("response: %s\n", string(resp))

	// Find by tag
	hits, err := rec.FindByTag(ctx, "env:dev")
	if err != nil {
		log.Fatalf("FindByTag failed: %v", err)
	}
	fmt.Printf("env:dev -> %v\n", hits)

	// Async usage
	async := rec.Async()
	if err := <-async.RecordMetrics(ctx, nil, "req-42", map[string]string{"duration_ms": "17"}, map[string]string{"env": "dev"}); err != nil {
		log.Fatalf("async RecordMetrics failed: %v", err)
	}
}
