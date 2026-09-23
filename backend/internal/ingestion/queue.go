package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const TaskTypeEnrich = "ingestion:enrich"

type taskPayload struct {
	RunID string `json:"run_id"`
	URL   string `json:"url"`
}

type Queue interface {
	Enqueue(context.Context, string, string) error
}

type AsynqQueue struct {
	client *asynq.Client
	queue  string
}

func NewAsynqQueue(client *asynq.Client, queue string) *AsynqQueue {
	return &AsynqQueue{client: client, queue: queue}
}

func (q *AsynqQueue) Enqueue(ctx context.Context, runID, sourceURL string) error {
	payload, err := json.Marshal(taskPayload{RunID: runID, URL: sourceURL})
	if err != nil {
		return fmt.Errorf("encode ingestion task: %w", err)
	}
	_, err = q.client.EnqueueContext(ctx, asynq.NewTask(TaskTypeEnrich, payload),
		asynq.Queue(q.queue), asynq.MaxRetry(3), asynq.Timeout(150*time.Second))
	if err != nil {
		return fmt.Errorf("enqueue ingestion task: %w", err)
	}
	return nil
}
