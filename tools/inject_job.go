package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
)

type ScrapeJob struct {
	VideoID  string `json:"video_id"`
	VideoURL string `json:"video_url"`
	Hashtag  string `json:"hashtag"`
	Desc     string `json:"desc"`
	Author   string `json:"author"`
	CommentID string `json:"comment_id"`
	Cursor   int    `json:"cursor"`
	Count    int    `json:"count"`
}

func main() {
	nc, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatal(err)
	}

	job := ScrapeJob{
		VideoID:  "7617886283213376798",
		VideoURL: "https://www.tiktok.com/@canyouspellcaio/video/7617886283213376798",
		Hashtag:  "test",
		Cursor:   0,
		Count:    20,
	}

	data, _ := json.Marshal(job)

	_, err = js.Publish("jobs.scrape", data)
	if err != nil {
		log.Fatal("Erro ao publicar:", err)
	}

	fmt.Println("✅ Payload injetado com sucesso no NATS (jobs.scrape)!")
	fmt.Println("Vídeo:", job.VideoID)
}
