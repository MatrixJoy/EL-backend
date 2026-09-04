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
	shutdownTimeout, err := duration("LEARNING_SHUTDOWN_TIMEOUT", "VOA_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	fetchTimeout, err := duration("LEARNING_FETCH_TIMEOUT", "VOA_FETCH_TIMEOUT", 20*time.Second)
	if err != nil {
		return Config{}, err
	}
	crawlDelay, err := duration("LEARNING_CRAWL_DELAY", "VOA_CRAWL_DELAY", 1500*time.Millisecond)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment:       value("LEARNING_ENV", "VOA_ENV", "development"),
		HTTPAddr:          value("LEARNING_HTTP_ADDR", "VOA_HTTP_ADDR", ":8080"),
		ReadHeaderTimeout: 5 * time.Second,
		ShutdownTimeout:   shutdownTimeout,
		FetchTimeout:      fetchTimeout,
		CrawlDelay:        crawlDelay,
		UserAgent:         value("LEARNING_USER_AGENT", "VOA_USER_AGENT", "EnglishLearningBackend/0.1 (+https://example.com/contact)"),
		DatabaseURL:       value("LEARNING_DATABASE_URL", "VOA_DATABASE_URL", "postgres://localhost:5432/voa_learning?sslmode=disable"),
		RedisAddr:         value("LEARNING_REDIS_ADDR", "VOA_REDIS_ADDR", "localhost:6379"),
		ObjectBucket:      value("LEARNING_OBJECT_BUCKET", "VOA_OBJECT_BUCKET", "voa-learning-raw"),
		PublishedBucket:   value("LEARNING_PUBLISHED_OBJECT_BUCKET", "VOA_PUBLISHED_OBJECT_BUCKET", "voa-learning-media"),
		ObjectEndpoint:    value("LEARNING_OBJECT_ENDPOINT", "VOA_OBJECT_ENDPOINT", "127.0.0.1:9000"),
		ObjectAccessKey:   value("LEARNING_OBJECT_ACCESS_KEY", "VOA_OBJECT_ACCESS_KEY", "minioadmin"),
		ObjectSecretKey:   value("LEARNING_OBJECT_SECRET_KEY", "VOA_OBJECT_SECRET_KEY", "minioadmin"),
		ObjectUseTLS:      value("LEARNING_OBJECT_USE_TLS", "VOA_OBJECT_USE_TLS", "false") == "true",
		AppleClientID:     value("LEARNING_APPLE_CLIENT_ID", "VOA_APPLE_CLIENT_ID", ""),
		CMSPublishToken:   value("LEARNING_CMS_PUBLISH_TOKEN", "VOA_CMS_PUBLISH_TOKEN", ""),
		SessionTTL:        30 * 24 * time.Hour,
	}, nil
}

func value(primary, legacy, fallback string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	if v := os.Getenv(legacy); v != "" {
		return v
	}
	return fallback
}

func duration(primary, legacy string, fallback time.Duration) (time.Duration, error) {
	v := value(primary, legacy, "")
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", primary, err)
	}
	return d, nil
}
