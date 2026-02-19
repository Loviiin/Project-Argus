package repository

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type Deduplicator struct {
	rdb *redis.Client
}

func NewDeduplicator(address, password string, db int) *Deduplicator {
	rdb := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       db,
	})
	return &Deduplicator{rdb: rdb}
}

func (d *Deduplicator) IsNew(ctx context.Context, url string) (bool, error) {
	key := fmt.Sprintf("argus:seen:%s", url)

	exists, err := d.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}

	return exists == 0, nil
}

func (d *Deduplicator) MarkAsSeen(ctx context.Context, url string) error {
	key := fmt.Sprintf("argus:seen:%s", url)
	_, err := d.rdb.Set(ctx, key, "1", 7*24*time.Hour).Result()
	return err
}

func (d *Deduplicator) Close() error {
	return d.rdb.Close()
}
