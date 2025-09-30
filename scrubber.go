package recorder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

type FieldContext struct {
	Path  []string
	Key   string
	Value any
}

type FieldMatcher func(FieldContext) bool

type ScrubFunc func(FieldContext) any

type Rule struct {
	Name     string
	Match    FieldMatcher
	Apply    ScrubFunc
	Continue bool
}

type ScrubberOption func(*Scrubber)

type Scrubber struct {
	rules              []Rule
	defaultReplacement string
	includeDefaults    bool
}

func NewScrubber(opts ...ScrubberOption) *Scrubber {
	s := &Scrubber{
		defaultReplacement: "[REDACTED]",
		includeDefaults:    true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	if s.includeDefaults {
		s.rules = append(s.rules, defaultSensitiveRules(s.defaultReplacement)...)
	}
	return s
}

func WithRules(rules ...Rule) ScrubberOption {
	return func(s *Scrubber) {
		if len(rules) == 0 {
			return
		}
		s.rules = append(s.rules, rules...)
	}
}

func WithoutDefaultRules() ScrubberOption {
	return func(s *Scrubber) {
		s.includeDefaults = false
	}
}

func WithDefaultReplacement(replacement string) ScrubberOption {
	return func(s *Scrubber) {
		if replacement == "" {
			return
		}
		s.defaultReplacement = replacement
	}
}

func (s *Scrubber) AddRules(rules ...Rule) {
	if s == nil || len(rules) == 0 {
		return
	}
	s.rules = append(s.rules, rules...)
}

func (s *Scrubber) Scrub(value any) any {
	if s == nil {
		return value
	}
	return s.scrubValue(value, nil)
}

func (s *Scrubber) ScrubMap(data map[string]any) map[string]any {
	if s == nil || data == nil {
		return data
	}
	if res, ok := s.scrubValue(data, nil).(map[string]any); ok {
		return res
	}
	return data
}

func (s *Scrubber) ScrubJSON(data []byte) ([]byte, error) {
	if s == nil {
		return append([]byte(nil), data...), nil
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return append([]byte(nil), data...), nil
	}
	var payload any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, fmt.Errorf("scrubber: decode json: %w", err)
	}
	scrubbed := s.scrubValue(payload, nil)
	result, err := json.Marshal(scrubbed)
	if err != nil {
		return nil, fmt.Errorf("scrubber: encode json: %w", err)
	}
	return result, nil
}

func (s *Scrubber) ScrubStringMap(data map[string]string) map[string]string {
	cloned := cloneTags(data)
	if s == nil || len(cloned) == 0 {
		return cloned
	}
	generic := make(map[string]any, len(cloned))
	for k, v := range cloned {
		generic[k] = v
	}
	scrubbed := s.scrubValue(generic, nil)
	result, ok := scrubbed.(map[string]any)
	if !ok {
		return cloned
	}
	final := make(map[string]string, len(result))
	for k, v := range result {
		final[k] = toString(v)
	}
	return final
}

func (s *Scrubber) scrubValue(value any, path []string) any {
	switch v := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(v))
		for k, val := range v {
			currentPath := append(path, k)
			ctx := FieldContext{Path: currentPath, Key: k, Value: val}
			if replaced, ok := s.applyRules(ctx); ok {
				cloned[k] = replaced
				continue
			}
			cloned[k] = s.scrubValue(val, currentPath)
		}
		return cloned
	case map[string]string:
		cloned := make(map[string]string, len(v))
		for k, val := range v {
			currentPath := append(path, k)
			ctx := FieldContext{Path: currentPath, Key: k, Value: val}
			if replaced, ok := s.applyRules(ctx); ok {
				cloned[k] = toString(replaced)
				continue
			}
			cloned[k] = toString(s.scrubValue(val, currentPath))
		}
		return cloned
	case map[string][]string:
		cloned := make(map[string][]string, len(v))
		for k, vals := range v {
			currentPath := append(path, k)
			ctx := FieldContext{Path: currentPath, Key: k, Value: vals}
			if replaced, ok := s.applyRules(ctx); ok {
				cloned[k] = toStringSlice(replaced, len(vals))
				continue
			}
			clonedVals := make([]string, len(vals))
			for i, item := range vals {
				elementPath := append(currentPath, fmt.Sprintf("[%d]", i))
				sanitized := s.scrubValue(item, elementPath)
				clonedVals[i] = toString(sanitized)
			}
			cloned[k] = clonedVals
		}
		return cloned
	case []string:
		cloned := make([]string, len(v))
		for i, val := range v {
			elementPath := append(path, fmt.Sprintf("[%d]", i))
			ctx := FieldContext{Path: elementPath, Key: fmt.Sprintf("[%d]", i), Value: val}
			if replaced, ok := s.applyRules(ctx); ok {
				cloned[i] = toString(replaced)
				continue
			}
			cloned[i] = toString(s.scrubValue(val, elementPath))
		}
		return cloned
	case []any:
		cloned := make([]any, len(v))
		for i, val := range v {
			cloned[i] = s.scrubValue(val, path)
		}
		return cloned
	default:
		return value
	}
}

func (s *Scrubber) applyRules(ctx FieldContext) (any, bool) {
	current := ctx.Value
	matched := false
	for _, rule := range s.rules {
		matcher := rule.Match
		if matcher != nil && !matcher(FieldContext{Path: ctx.Path, Key: ctx.Key, Value: current}) {
			continue
		}
		if rule.Apply == nil {
			continue
		}
		matched = true
		current = rule.Apply(FieldContext{Path: ctx.Path, Key: ctx.Key, Value: current})
		if !rule.Continue {
			break
		}
	}
	if matched {
		return current, true
	}
	return nil, false
}

func defaultSensitiveRules(replacement string) []Rule {
	matcher := MatchKeyInsensitive(
		"password",
		"pass",
		"passwd",
		"secret",
		"token",
		"auth",
		"authorization",
		"api_key",
		"apikey",
		"access_token",
		"refresh_token",
		"session",
		"cookie",
		"set-cookie",
	)
	return []Rule{
		{
			Name:  "default-sensitive",
			Match: matcher,
			Apply: ReplaceWith(replacement),
		},
	}
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func toStringSlice(value any, desiredLen int) []string {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []string:
		out := make([]string, len(v))
		copy(out, v)
		return out
	case []any:
		out := make([]string, len(v))
		for i, item := range v {
			out[i] = toString(item)
		}
		return out
	case string:
		if desiredLen <= 0 {
			return []string{v}
		}
		out := make([]string, desiredLen)
		for i := range out {
			out[i] = v
		}
		return out
	default:
		valueStr := toString(v)
		if desiredLen <= 0 {
			return []string{valueStr}
		}
		out := make([]string, desiredLen)
		for i := range out {
			out[i] = valueStr
		}
		return out
	}
}

func MatchAny(matchers ...FieldMatcher) FieldMatcher {
	return func(ctx FieldContext) bool {
		for _, matcher := range matchers {
			if matcher != nil && matcher(ctx) {
				return true
			}
		}
		return false
	}
}

func MatchAll(matchers ...FieldMatcher) FieldMatcher {
	return func(ctx FieldContext) bool {
		for _, matcher := range matchers {
			if matcher == nil {
				continue
			}
			if !matcher(ctx) {
				return false
			}
		}
		return true
	}
}

func MatchNot(matcher FieldMatcher) FieldMatcher {
	return func(ctx FieldContext) bool {
		if matcher == nil {
			return false
		}
		return !matcher(ctx)
	}
}

func MatchKeyInsensitive(keys ...string) FieldMatcher {
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, strings.ToLower(trimmed))
	}
	return func(ctx FieldContext) bool {
		key := strings.ToLower(ctx.Key)
		for _, candidate := range normalized {
			if key == candidate {
				return true
			}
		}
		return false
	}
}

func MatchKeyPrefixInsensitive(prefixes ...string) FieldMatcher {
	normalized := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		trimmed := strings.TrimSpace(prefix)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, strings.ToLower(trimmed))
	}
	return func(ctx FieldContext) bool {
		key := strings.ToLower(ctx.Key)
		for _, prefix := range normalized {
			if strings.HasPrefix(key, prefix) {
				return true
			}
		}
		return false
	}
}

func MatchKeyContainsInsensitive(parts ...string) FieldMatcher {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, strings.ToLower(trimmed))
	}
	return func(ctx FieldContext) bool {
		key := strings.ToLower(ctx.Key)
		for _, part := range normalized {
			if strings.Contains(key, part) {
				return true
			}
		}
		return false
	}
}

func MatchPathInsensitive(paths ...string) FieldMatcher {
	patterns := make([][]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		segments := strings.Split(trimmed, ".")
		for i, segment := range segments {
			segments[i] = strings.TrimSpace(segment)
		}
		patterns = append(patterns, segments)
	}
	return func(ctx FieldContext) bool {
		for _, pattern := range patterns {
			if pathMatches(ctx.Path, pattern) {
				return true
			}
		}
		return false
	}
}

func pathMatches(path []string, pattern []string) bool {
	if len(path) != len(pattern) {
		return false
	}
	for i, segment := range pattern {
		if segment == "*" {
			continue
		}
		if !strings.EqualFold(path[i], segment) {
			return false
		}
	}
	return true
}

func MatchKeyRegexp(re *regexp.Regexp) FieldMatcher {
	return func(ctx FieldContext) bool {
		if re == nil {
			return false
		}
		return re.MatchString(ctx.Key)
	}
}

func MatchValueRegexp(re *regexp.Regexp) FieldMatcher {
	return func(ctx FieldContext) bool {
		if re == nil || ctx.Value == nil {
			return false
		}
		return re.MatchString(fmt.Sprint(ctx.Value))
	}
}

func ReplaceWith(replacement string) ScrubFunc {
	return func(FieldContext) any {
		return replacement
	}
}

func RemoveValue() ScrubFunc {
	return func(FieldContext) any {
		return nil
	}
}

func MaskString(mask rune, keepStart, keepEnd int) ScrubFunc {
	if mask == 0 {
		mask = '*'
	}
	if keepStart < 0 {
		keepStart = 0
	}
	if keepEnd < 0 {
		keepEnd = 0
	}
	maskStr := string(mask)
	return func(ctx FieldContext) any {
		if ctx.Value == nil {
			return nil
		}
		value := fmt.Sprint(ctx.Value)
		if value == "" {
			return value
		}
		runes := []rune(value)
		length := len(runes)
		if length == 0 {
			return value
		}
		if keepStart+keepEnd >= length {
			return strings.Repeat(maskStr, length)
		}
		builder := strings.Builder{}
		builder.Grow(length)
		if keepStart > 0 {
			builder.WriteString(string(runes[:keepStart]))
		}
		maskedCount := length - keepStart - keepEnd
		builder.WriteString(strings.Repeat(maskStr, maskedCount))
		if keepEnd > 0 {
			builder.WriteString(string(runes[length-keepEnd:]))
		}
		return builder.String()
	}
}

func PreserveLengthReplacement(replacement string) ScrubFunc {
	maskSource := replacement
	if maskSource == "" {
		maskSource = "*"
	}
	return func(ctx FieldContext) any {
		if ctx.Value == nil {
			return maskSource
		}
		value := fmt.Sprint(ctx.Value)
		if value == "" {
			return maskSource
		}
		maskRune, _ := utf8.DecodeRuneInString(maskSource)
		if maskRune == utf8.RuneError {
			maskRune = '*'
		}
		return strings.Repeat(string(maskRune), utf8.RuneCountInString(value))
	}
}

func NewRule(name string, matcher FieldMatcher, scrub ScrubFunc) Rule {
	return Rule{Name: name, Match: matcher, Apply: scrub}
}
