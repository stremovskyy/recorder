package redis_recorder

import "time"

type Options struct {
	Debug          bool
	Addr           string
	Password       string
	DB             int
	DefaultTTL     time.Duration
	CompressionLvl int
	Prefix         string
}

func NewDefaultOptions(addr string, password string, DB int) *Options {
	return &Options{
		Addr:           addr,
		Password:       password,
		DB:             DB,
		DefaultTTL:     24 * time.Hour * 7, // one week
		CompressionLvl: 3,
		Prefix:         "fondy:http",
	}
}
