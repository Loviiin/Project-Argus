package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"discovery/internal/sources/tiktok"
	"github.com/loviiin/project-argus/pkg/config"
)

type commentResponse struct {
	Comments []struct {
		Text string `json:"text"`
		User struct {
			UniqueID string `json:"unique_id"`
		} `json:"user"`
	} `json:"comments"`
	Cursor     int    `json:"cursor"`
	HasMore    int    `json:"has_more"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error"`
}

func main() {
	videoURL := flag.String("url", "https://www.tiktok.com/@_headlessgirl/video/7620562518834007303?q=discord&t=1779241076610o", "TikTok video URL")
	cursor := flag.String("cursor", "0", "comment cursor")
	count := flag.String("count", "20", "comment count")
	currentRegion := flag.String("current-region", "US", "comment current region")
	sidecar := flag.String("sidecar", "", "sidecar url (defaults to config)")
	ttwid := flag.String("ttwid", "", "ttwid override (defaults to config)")
	msToken := flag.String("mstoken", "", "msToken override (optional)")
	flag.Parse()

	cfg := config.LoadConfig()
	resolvedSidecar := strings.TrimSpace(*sidecar)
	if resolvedSidecar == "" {
		resolvedSidecar = strings.TrimSpace(cfg.TikTok.SidecarURL)
	}
	if resolvedSidecar == "" {
		resolvedSidecar = "http://localhost:8000"
	}

	resolvedTtwid := strings.TrimSpace(*ttwid)
	if resolvedTtwid == "" {
		resolvedTtwid = strings.TrimSpace(cfg.TikTok.Ttwid)
	}

	videoID := tiktok.ExtractVideoID(*videoURL)
	if videoID == "" {
		log.Fatalf("could not extract video id from %q", *videoURL)
	}

	payload := map[string]string{
		"video_id":       videoID,
		"video_url":      *videoURL,
		"cursor":         *cursor,
		"count":          *count,
		"current_region": *currentRegion,
	}
	if resolvedTtwid != "" {
		payload["ttwid"] = resolvedTtwid
	}
	if token := strings.TrimSpace(*msToken); token != "" {
		payload["msToken"] = token
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("marshal request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(resolvedSidecar, "/")+"/comments", bytes.NewReader(body))
	if err != nil {
		log.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("call sidecar: %v", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("read response: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Fatalf("sidecar returned %s: %s", resp.Status, strings.TrimSpace(string(rawBody)))
	}

	var parsed commentResponse
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		fmt.Printf("status=%s\n", resp.Status)
		fmt.Printf("raw=%s\n", strings.TrimSpace(string(rawBody)))
		return
	}

	fmt.Printf("status=%s video_id=%s comments=%d cursor=%d has_more=%d\n", resp.Status, videoID, len(parsed.Comments), parsed.Cursor, parsed.HasMore)
	for i, comment := range parsed.Comments {
		if i >= 5 {
			break
		}
		fmt.Printf("%d. @%s: %s\n", i+1, comment.User.UniqueID, comment.Text)
	}
}
