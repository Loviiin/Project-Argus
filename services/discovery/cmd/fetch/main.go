package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"discovery/internal/sources"
)

func main() {
	sidecar := flag.String("sidecar", "http://localhost:8000", "sidecar url")
	ttwid := flag.String("ttwid", "", "ttwid cookie (optional)")
	hashtag := flag.String("hashtag", "discord", "hashtag to search (without #)")
	flag.Parse()

	src := sources.NewTikTokHTTPSource(*sidecar, *ttwid, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	videos, err := src.Fetch(ctx, *hashtag)
	if err != nil {
		log.Fatalf("Fetch failed: %v", err)
	}

	fmt.Printf("Found %d videos:\n", len(videos))
	for i, v := range videos {
		fmt.Printf("%d: %s (%s)\n", i+1, v.URL, v.ID)
	}
}
