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

	created, err := d.rdb.SetNX(ctx, key, "1", 7*24*time.Hour).Result()
	if err != nil {
		return false, err
	}

	return created, nil
}

func (d *Deduplicator) Close() error {
	return d.rdb.Close()
}
