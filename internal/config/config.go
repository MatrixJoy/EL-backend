package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Environment       string
	HTTPAddr          string
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
	FetchTimeout      time.Duration
	CrawlDelay        time.Duration
	UserAgent         string
	DatabaseURL       string
	RedisAddr         string
	ObjectBucket      string
	PublishedBucket   string
	ObjectEndpoint    string
	ObjectAccessKey   string
	ObjectSecretKey   string
	ObjectUseTLS      bool
	AppleClientID     string
	CMSPublishToken   string
	SessionTTL        time.Duration
}

func Load() (Config, error) {
	shutdownTimeout, err := duration("VOA_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	fetchTimeout, err := duration("VOA_FETCH_TIMEOUT", 20*time.Second)
	if err != nil {
		return Config{}, err
	}
	crawlDelay, err := duration("VOA_CRAWL_DELAY", 1500*time.Millisecond)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment:       value("VOA_ENV", "development"),
		HTTPAddr:          value("VOA_HTTP_ADDR", ":8080"),
		ReadHeaderTimeout: 5 * time.Second,
		ShutdownTimeout:   shutdownTimeout,
		FetchTimeout:      fetchTimeout,
		CrawlDelay:        crawlDelay,
		UserAgent:         value("VOA_USER_AGENT", "VOALearningApp/0.1 (+https://example.com/contact)"),
		DatabaseURL:       value("VOA_DATABASE_URL", "postgres://localhost:5432/voa_learning?sslmode=disable"),
		RedisAddr:         value("VOA_REDIS_ADDR", "localhost:6379"),
		ObjectBucket:      value("VOA_OBJECT_BUCKET", "voa-learning-raw"),
		PublishedBucket:   value("VOA_PUBLISHED_OBJECT_BUCKET", "voa-learning-media"),
		ObjectEndpoint:    value("VOA_OBJECT_ENDPOINT", "127.0.0.1:9000"),
		ObjectAccessKey:   value("VOA_OBJECT_ACCESS_KEY", "minioadmin"),
		ObjectSecretKey:   value("VOA_OBJECT_SECRET_KEY", "minioadmin"),
		ObjectUseTLS:      value("VOA_OBJECT_USE_TLS", "false") == "true",
		AppleClientID:     os.Getenv("VOA_APPLE_CLIENT_ID"),
		CMSPublishToken:   os.Getenv("VOA_CMS_PUBLISH_TOKEN"),
		SessionTTL:        30 * 24 * time.Hour,
	}, nil
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return d, nil
}
