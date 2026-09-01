package ingestion

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TaskProcessSource = "ingestion:process_source"
const TaskDiscoverSitemap = "ingestion:discover_sitemap"

type ProcessSourcePayload struct {
	SourceID uuid.UUID `json:"sourceId"`
}

type DiscoverSitemapPayload struct {
	URL string `json:"url"`
}

func NewDiscoverSitemapTask(sourceURL string) (*asynq.Task, error) {
	payload, err := json.Marshal(DiscoverSitemapPayload{URL: sourceURL})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskDiscoverSitemap, payload), nil
}

func DiscoverSitemapHandler(discoverer *Discoverer) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		var payload DiscoverSitemapPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("decode payload: %w", asynq.SkipRetry)
		}
		_, err := discoverer.Discover(ctx, payload.URL)
		return err
	}
}

func NewProcessSourceTask(sourceID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(ProcessSourcePayload{SourceID: sourceID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskProcessSource, payload), nil
}

func ProcessSourceHandler(pipeline *Pipeline) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		var payload ProcessSourcePayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("decode payload: %w", asynq.SkipRetry)
		}
		if payload.SourceID == uuid.Nil {
			return fmt.Errorf("missing source ID: %w", asynq.SkipRetry)
		}
		return pipeline.Process(ctx, payload.SourceID)
	}
}
