package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Deduplicator handles Redis deduplication checks across services
type Deduplicator struct {
	rdb      *redis.Client
	ttlHours int
}

// NewDeduplicator creates a new shared instance. If ttlHours is 0, defaults to 48 hours.
func NewDeduplicator(rdb *redis.Client, ttlHours int) *Deduplicator {
	if ttlHours <= 0 {
		ttlHours = 48
	}
	return &Deduplicator{
		rdb:      rdb,
		ttlHours: ttlHours,
	}
}

// MarkAsSeen marks an entity string (like a video ID) as seen under a specific prefix type (e.g. "seen", "processed")
func (d *Deduplicator) MarkAsSeen(ctx context.Context, prefixType string, id string) error {
	key := fmt.Sprintf("argus:%s:%s", prefixType, id)
	ttl := time.Duration(d.ttlHours) * time.Hour
	_, err := d.rdb.Set(ctx, key, "1", ttl).Result()
	return err
}

// CheckIfProcessed returns true if the entity string exists under the prefix type
func (d *Deduplicator) CheckIfProcessed(ctx context.Context, prefixType string, id string) (bool, error) {
	key := fmt.Sprintf("argus:%s:%s", prefixType, id)
	exists, err := d.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// RDB returns the internal redis client for external lock usages
func (d *Deduplicator) RDB() *redis.Client {
	return d.rdb
}

// Close closes the underlying redis connection
func (d *Deduplicator) Close() error {
	return d.rdb.Close()
}
