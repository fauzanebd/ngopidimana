package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrNotFound = errors.New("ingestion run not found")

type Store interface {
	List(context.Context) ([]Run, error)
	Get(context.Context, string) (Run, error)
	Save(context.Context, Run) error
	Delete(context.Context, string) error
}

type RedisStore struct {
	client *redis.Client
	prefix string
}

func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client, prefix: "wfc:ingestion"}
}

func (s *RedisStore) key(id string) string { return s.prefix + ":run:" + id }

func (s *RedisStore) deletedKey(id string) string { return s.prefix + ":deleted:" + id }

func (s *RedisStore) Save(ctx context.Context, run Run) error {
	payload, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("marshal ingestion run: %w", err)
	}
	result, err := s.client.Eval(ctx, `
		if redis.call("EXISTS", KEYS[2]) == 1 then
			return 0
		end
		redis.call("SET", KEYS[1], ARGV[1])
		redis.call("ZADD", KEYS[3], ARGV[2], ARGV[3])
		return 1
	`, []string{s.key(run.ID), s.deletedKey(run.ID), s.prefix + ":runs"}, payload, run.CreatedAt.UnixMilli(), run.ID).Int()
	if err != nil {
		return fmt.Errorf("save ingestion run: %w", err)
	}
	if result == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *RedisStore) Delete(ctx context.Context, id string) error {
	result, err := s.client.Eval(ctx, `
		if redis.call("EXISTS", KEYS[1]) == 0 then
			return 0
		end
		redis.call("SET", KEYS[2], "1", "EX", 86400)
		redis.call("DEL", KEYS[1])
		redis.call("ZREM", KEYS[3], ARGV[1])
		return 1
	`, []string{s.key(id), s.deletedKey(id), s.prefix + ":runs"}, id).Int()
	if err != nil {
		return fmt.Errorf("delete ingestion run: %w", err)
	}
	if result == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *RedisStore) Get(ctx context.Context, id string) (Run, error) {
	payload, err := s.client.Get(ctx, s.key(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("get ingestion run: %w", err)
	}
	var run Run
	if err := json.Unmarshal(payload, &run); err != nil {
		return Run{}, fmt.Errorf("decode ingestion run: %w", err)
	}
	return run, nil
}

func (s *RedisStore) List(ctx context.Context) ([]Run, error) {
	ids, err := s.client.ZRevRange(ctx, s.prefix+":runs", 0, 199).Result()
	if err != nil {
		return nil, fmt.Errorf("list ingestion run ids: %w", err)
	}
	if len(ids) == 0 {
		return []Run{}, nil
	}
	keys := make([]string, len(ids))
	for index, id := range ids {
		keys[index] = s.key(id)
	}
	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("list ingestion runs: %w", err)
	}
	runs := make([]Run, 0, len(values))
	for _, value := range values {
		encoded, ok := value.(string)
		if !ok {
			continue
		}
		var run Run
		if json.Unmarshal([]byte(encoded), &run) == nil {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

type MemoryStore struct {
	mu      sync.RWMutex
	runs    map[string]Run
	deleted map[string]time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{runs: map[string]Run{}, deleted: map[string]time.Time{}}
}

func (s *MemoryStore) Save(_ context.Context, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, deleted := s.deleted[run.ID]; deleted {
		return ErrNotFound
	}
	s.runs[run.ID] = run
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[id]; !exists {
		return ErrNotFound
	}
	delete(s.runs, id)
	s.deleted[id] = time.Now()
	return nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[id]
	if !ok {
		return Run{}, ErrNotFound
	}
	return run, nil
}

func (s *MemoryStore) List(_ context.Context) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	runs := make([]Run, 0, len(s.runs))
	for _, run := range s.runs {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	return runs, nil
}
