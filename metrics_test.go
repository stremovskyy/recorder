package recorder

import (
	"testing"
	"time"
)

func TestMetricsCountersAndGauges(t *testing.T) {
	m := NewMetrics()

	m.IncrementCounter("requests", map[string]string{"type": "test"})
	m.IncrementCounter("requests", map[string]string{"type": "test"})
	m.SetGauge("queue_length", 42, map[string]string{"queue": "a"})

	counters := m.GetCounters()
	if len(counters) != 1 {
		t.Fatalf("expected 1 counter entry, got %d", len(counters))
	}
	for _, value := range counters {
		if value != 2 {
			t.Fatalf("expected counter value 2, got %d", value)
		}
	}

	gauges := m.GetGauges()
	if len(gauges) != 1 {
		t.Fatalf("expected 1 gauge entry, got %d", len(gauges))
	}
	for _, value := range gauges {
		if value != 42 {
			t.Fatalf("expected gauge value 42, got %f", value)
		}
	}
}

func TestMetricsHistogramAndTiming(t *testing.T) {
	m := NewMetrics()

	m.RecordHistogram("latency", 10.5, map[string]string{"endpoint": "a"})
	m.RecordTiming("latency", 1500*time.Millisecond, map[string]string{"endpoint": "a"})

	histograms := m.GetHistograms()
	if len(histograms) != 1 {
		t.Fatalf("expected histogram entry, got %d", len(histograms))
	}

	var values []float64
	for _, v := range histograms {
		values = v
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 recorded values, got %d", len(values))
	}
	if values[0] != 10.5 {
		t.Fatalf("expected first histogram value 10.5, got %f", values[0])
	}
	if values[1] != 1500 {
		t.Fatalf("expected timing conversion to 1500 ms, got %f", values[1])
	}

	values[0] = 0 // mutate copy
	histograms = m.GetHistograms()
	for _, v := range histograms {
		if v[0] != 10.5 {
			t.Fatal("expected histogram copy to remain unchanged")
		}
	}
}

func TestMetricsKeyDeterministic(t *testing.T) {
	m := NewMetrics()
	m.IncrementCounter("requests", map[string]string{"b": "2", "a": "1"})
	m.IncrementCounter("requests", map[string]string{"a": "1", "b": "2"})

	counters := m.GetCounters()
	if len(counters) != 1 {
		t.Fatalf("expected single counter entry, got %d", len(counters))
	}
	for key, value := range counters {
		if value != 2 {
			t.Fatalf("expected combined counter value 2, got %d", value)
		}
		if key != "requests,a=1,b=2" {
			t.Fatalf("unexpected key ordering: %s", key)
		}
	}
}
