package recorder

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestScrubber_DefaultRules(t *testing.T) {
	t.Parallel()

	scrub := NewScrubber()
	input := map[string]any{
		"password": "secret",
		"profile": map[string]any{
			"token": "abc123",
			"name":  "john",
		},
		"headers": map[string][]string{
			"Authorization": []string{"Bearer abc", "Token def"},
			"X-Trace":       []string{"trace"},
		},
		"entries": []any{
			map[string]any{"auth": "value", "ok": "keep"},
			"plain",
		},
	}

	resultAny := scrub.Scrub(input)
	result, ok := resultAny.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any result, got %T", resultAny)
	}

	if got := result["password"]; got != "[REDACTED]" {
		t.Fatalf("password not scrubbed, got %v", got)
	}

	nested, ok := result["profile"].(map[string]any)
	if !ok {
		t.Fatalf("profile type changed, got %T", result["profile"])
	}
	if nested["token"] != "[REDACTED]" {
		t.Fatalf("token not scrubbed, got %v", nested["token"])
	}
	if nested["name"] != "john" {
		t.Fatalf("non-sensitive field altered, got %v", nested["name"])
	}

	headers, ok := result["headers"].(map[string][]string)
	if !ok {
		t.Fatalf("headers type changed, got %T", result["headers"])
	}
	wantAuth := []string{"[REDACTED]", "[REDACTED]"}
	if !reflect.DeepEqual(headers["Authorization"], wantAuth) {
		t.Fatalf("authorization scrub mismatch: %v", headers["Authorization"])
	}
	if !reflect.DeepEqual(headers["X-Trace"], []string{"trace"}) {
		t.Fatalf("non-sensitive header changed: %v", headers["X-Trace"])
	}

	entries, ok := result["entries"].([]any)
	if !ok {
		t.Fatalf("entries type changed, got %T", result["entries"])
	}
	nestedEntry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("entries[0] type changed, got %T", entries[0])
	}
	if nestedEntry["auth"] != "[REDACTED]" {
		t.Fatalf("auth entry not scrubbed, got %v", nestedEntry["auth"])
	}
	if nestedEntry["ok"] != "keep" {
		t.Fatalf("non-sensitive entry altered, got %v", nestedEntry["ok"])
	}
}

func TestScrubber_CustomRules(t *testing.T) {
	t.Parallel()

	scrub := NewScrubber(
		WithoutDefaultRules(),
	)
	scrub.AddRules(
		NewRule("mask-token", MatchPathInsensitive("credentials.token"), MaskString('*', 0, 4)),
		NewRule("hide-auth-header", MatchKeyContainsInsensitive("auth"), ReplaceWith("<hidden>")),
	)

	input := map[string]any{
		"credentials": map[string]any{
			"token": "1234567890",
			"role":  "admin",
		},
		"Authorization": "Bearer token",
	}

	result := scrub.Scrub(input).(map[string]any)

	creds := result["credentials"].(map[string]any)
	if creds["token"] != "******7890" {
		t.Fatalf("token masking failed, got %v", creds["token"])
	}
	if creds["role"] != "admin" {
		t.Fatalf("role unexpectedly changed, got %v", creds["role"])
	}

	if result["Authorization"] != "<hidden>" {
		t.Fatalf("authorization header not replaced, got %v", result["Authorization"])
	}
}

func TestScrubber_ScrubJSON(t *testing.T) {
	t.Parallel()

	scrub := NewScrubber()
	input := []byte(`{"password":"secret","nested":{"token":"abc","other":true}}`)

	output, err := scrub.ScrubJSON(input)
	if err != nil {
		t.Fatalf("ScrubJSON error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	if result["password"] != "[REDACTED]" {
		t.Fatalf("password not scrubbed: %v", result["password"])
	}

	nested := result["nested"].(map[string]any)
	if nested["token"] != "[REDACTED]" {
		t.Fatalf("nested token not scrubbed: %v", nested["token"])
	}
	if nested["other"] != true {
		t.Fatalf("non-sensitive value changed: %v", nested["other"])
	}
}

func TestScrubber_HeaderSlicePreservesLength(t *testing.T) {
	t.Parallel()

	scrub := NewScrubber()
	headers := map[string][]string{
		"Authorization": []string{"Bearer 1", "Bearer 2"},
		"Cookie":        []string{"name=value"},
		"X-Trace":       []string{"trace"},
	}

	resultAny := scrub.Scrub(headers)
	result, ok := resultAny.(map[string][]string)
	if !ok {
		t.Fatalf("expected map[string][]string, got %T", resultAny)
	}

	if got := result["Authorization"]; !reflect.DeepEqual(got, []string{"[REDACTED]", "[REDACTED]"}) {
		t.Fatalf("authorization not scrubbed correctly: %v", got)
	}

	if got := result["Cookie"]; !reflect.DeepEqual(got, []string{"[REDACTED]"}) {
		t.Fatalf("cookie not scrubbed correctly: %v", got)
	}

	if got := result["X-Trace"]; !reflect.DeepEqual(got, []string{"trace"}) {
		t.Fatalf("non-sensitive header changed: %v", got)
	}
}

func TestScrubber_ScrubStringMap(t *testing.T) {
	t.Parallel()

	scrub := NewScrubber()
	input := map[string]string{"token": "abc", "other": "ok"}

	result := scrub.ScrubStringMap(input)
	if input["token"] != "abc" {
		t.Fatalf("expected original map to remain unchanged, got %q", input["token"])
	}
	if result["token"] != "[REDACTED]" {
		t.Fatalf("expected token to be scrubbed, got %q", result["token"])
	}
	if result["other"] != "ok" {
		t.Fatalf("expected other key to remain, got %q", result["other"])
	}
}
