package recorder

type PayloadScrubFunc func(recordType RecordType, payload []byte) ([]byte, error)

type TagScrubFunc func(recordType RecordType, tags map[string]string) (map[string]string, error)

type RecorderOption func(*recorderOptions)

type recorderOptions struct {
	payloadScrubber PayloadScrubFunc
	tagScrubber     TagScrubFunc
}

func WithPayloadScrubber(fn PayloadScrubFunc) RecorderOption {
	return func(o *recorderOptions) {
		o.payloadScrubber = fn
	}
}

func WithTagScrubber(fn TagScrubFunc) RecorderOption {
	return func(o *recorderOptions) {
		o.tagScrubber = fn
	}
}

type ScrubberBindingOption func(*scrubberBindingConfig)

type scrubberBindingConfig struct {
	failOnError bool
	scrubTags   bool
}

func WithScrubber(scrubber *Scrubber, opts ...ScrubberBindingOption) RecorderOption {
	return func(o *recorderOptions) {
		if scrubber == nil {
			return
		}
		cfg := scrubberBindingConfig{scrubTags: true}
		for _, opt := range opts {
			if opt != nil {
				opt(&cfg)
			}
		}
		if scrubber != nil {
			o.payloadScrubber = func(recordType RecordType, payload []byte) ([]byte, error) {
				if len(payload) == 0 {
					return payload, nil
				}
				sanitized, err := scrubber.ScrubJSON(payload)
				if err != nil {
					if cfg.failOnError {
						return nil, err
					}
					return payload, nil
				}
				return sanitized, nil
			}
			if cfg.scrubTags {
				o.tagScrubber = func(recordType RecordType, tags map[string]string) (map[string]string, error) {
					return scrubber.ScrubStringMap(tags), nil
				}
			}
		}
	}
}

func ScrubberFailOnError() ScrubberBindingOption {
	return func(cfg *scrubberBindingConfig) {
		cfg.failOnError = true
	}
}

func ScrubberSkipTags() ScrubberBindingOption {
	return func(cfg *scrubberBindingConfig) {
		cfg.scrubTags = false
	}
}
