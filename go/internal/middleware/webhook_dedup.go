package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

// UpdateDeduper tracks processed Telegram update IDs.
type UpdateDeduper interface {
	Seen(ctx context.Context, updateID int64) (bool, error)
}

type redisUpdateDeduper struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

func (d *redisUpdateDeduper) Seen(ctx context.Context, updateID int64) (bool, error) {
	key := d.prefix + ":" + strconv.FormatInt(updateID, 10)
	ok, err := d.client.SetNX(ctx, key, "1", d.ttl).Result()
	if err != nil {
		return false, err
	}
	// false => already exists => duplicate
	return !ok, nil
}

type memoryUpdateDeduper struct {
	mu     sync.Mutex
	seen   map[int64]time.Time
	ttl    time.Duration
	nextGC time.Time
}

func newMemoryUpdateDeduper(ttl time.Duration) *memoryUpdateDeduper {
	now := time.Now()
	return &memoryUpdateDeduper{
		seen:   make(map[int64]time.Time),
		ttl:    ttl,
		nextGC: now.Add(ttl),
	}
}

func (d *memoryUpdateDeduper) Seen(_ context.Context, updateID int64) (bool, error) {
	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()

	if exp, ok := d.seen[updateID]; ok && exp.After(now) {
		return true, nil
	}

	d.seen[updateID] = now.Add(d.ttl)
	if now.After(d.nextGC) {
		for id, exp := range d.seen {
			if exp.Before(now) {
				delete(d.seen, id)
			}
		}
		d.nextGC = now.Add(d.ttl)
	}

	return false, nil
}

// NewUpdateDeduper builds a Redis deduper and falls back to in-memory on failure.
func NewUpdateDeduper(addr, pass string, db int, ttl time.Duration) (UpdateDeduper, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if addr == "" {
		return newMemoryUpdateDeduper(ttl), nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pass,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return newMemoryUpdateDeduper(ttl), err
	}

	return &redisUpdateDeduper{
		client: client,
		prefix: "tg:update",
		ttl:    ttl,
	}, nil
}

// TelegramUpdateDedup drops duplicate Telegram webhook updates by update_id.
func TelegramUpdateDedup(deduper UpdateDeduper) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if deduper == nil {
				return next(c)
			}

			req := c.Request()
			if req.Body == nil {
				return next(c)
			}

			rawBody, err := io.ReadAll(req.Body)
			if err != nil {
				return next(c)
			}
			req.Body = io.NopCloser(bytes.NewBuffer(rawBody))
			if len(rawBody) == 0 {
				return next(c)
			}

			var payload struct {
				UpdateID int64 `json:"update_id"`
			}
			if err := json.Unmarshal(rawBody, &payload); err != nil || payload.UpdateID == 0 {
				return next(c)
			}

			isDuplicate, err := deduper.Seen(req.Context(), payload.UpdateID)
			if err != nil {
				return next(c)
			}
			if isDuplicate {
				// Telegram only needs a 2xx response to stop retries.
				return c.NoContent(http.StatusOK)
			}

			return next(c)
		}
	}
}
