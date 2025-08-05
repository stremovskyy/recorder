package recorder

import (
	"sync"
	"time"
)

type Metrics interface {
	IncrementCounter(name string, tags map[string]string)
	SetGauge(name string, value float64, tags map[string]string)
	RecordHistogram(name string, value float64, tags map[string]string)
	RecordTiming(name string, duration time.Duration, tags map[string]string)
	GetCounters() map[string]int64
	GetGauges() map[string]float64
	GetHistograms() map[string][]float64
}

type inMemoryMetrics struct {
	mu         sync.RWMutex
	counters   map[string]int64
	gauges     map[string]float64
	histograms map[string][]float64
}

func NewMetrics() Metrics {
	return &inMemoryMetrics{
		counters:   make(map[string]int64),
		gauges:     make(map[string]float64),
		histograms: make(map[string][]float64),
	}
}

func (m *inMemoryMetrics) IncrementCounter(name string, tags map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, tags)
	m.counters[key]++
}

func (m *inMemoryMetrics) SetGauge(name string, value float64, tags map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, tags)
	m.gauges[key] = value
}

func (m *inMemoryMetrics) RecordHistogram(name string, value float64, tags map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, tags)
	m.histograms[key] = append(m.histograms[key], value)
}

func (m *inMemoryMetrics) RecordTiming(name string, duration time.Duration, tags map[string]string) {
	m.RecordHistogram(name, float64(duration.Nanoseconds())/1e6, tags) // Convert to milliseconds
}

func (m *inMemoryMetrics) GetCounters() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]int64)
	for k, v := range m.counters {
		result[k] = v
	}
	return result
}

func (m *inMemoryMetrics) GetGauges() map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]float64)
	for k, v := range m.gauges {
		result[k] = v
	}
	return result
}

func (m *inMemoryMetrics) GetHistograms() map[string][]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]float64)
	for k, v := range m.histograms {
		result[k] = make([]float64, len(v))
		copy(result[k], v)
	}
	return result
}

func (m *inMemoryMetrics) buildKey(name string, tags map[string]string) string {
	key := name
	for k, v := range tags {
		key += "," + k + "=" + v
	}
	return key
}
