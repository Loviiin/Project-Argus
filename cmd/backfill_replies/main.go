package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

type ScrapeJob struct {
	VideoID   string `json:"video_id"`
	VideoURL  string `json:"video_url"`
	Hashtag   string `json:"hashtag"`
	Desc      string `json:"desc"`
	Author    string `json:"author"`
	CommentID string `json:"comment_id"`
	Cursor    int    `json:"cursor"`
	Count     int    `json:"count"`
}

func main() {
	// 1. Conexão com NATS
	nc, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		log.Fatal("Erro conectando ao NATS:", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatal("Erro JetStream:", err)
	}

	// 2. Conexão com PostgreSQL (banco local mapeado no docker-compose)
	db, err := sql.Open("postgres", "postgres://argus-user:change_me@localhost:5432/argus-post-db?sslmode=disable")
	if err != nil {
		log.Fatal("Erro conectando ao DB:", err)
	}
	defer db.Close()

	// 3. Buscar comentários que têm respostas e possivelmente não foram raspados
	rows, err := db.Query(`
		SELECT aweme_id, cid, reply_comment_total 
		FROM comments 
		WHERE reply_comment_total > 0 
		  AND (reply_id IS NULL OR reply_id = '')
	`)
	if err != nil {
		log.Fatal("Erro na query:", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var videoID, cid string
		var replyTotal int
		if err := rows.Scan(&videoID, &cid, &replyTotal); err != nil {
			log.Println("Erro lendo linha:", err)
			continue
		}

		// 4. Montar e publicar o Job para a fila de replies
		job := ScrapeJob{
			VideoID:   videoID,
			CommentID: cid,
			Cursor:    0,
			Count:     20, // Padrão
		}

		data, _ := json.Marshal(job)
		_, err = js.Publish("jobs.scrape.reply", data)
		if err != nil {
			log.Printf("❌ Erro injetando reply do comentário %s: %v\n", cid, err)
		} else {
			fmt.Printf("✅ Injetado backfill para Comentário %s (Vídeo %s) - %d respostas\n", cid, videoID, replyTotal)
			count++
		}
	}

	fmt.Printf("\n🚀 Backfill concluído! %d comentários com respostas foram enviados para o Reply Worker!\n", count)
}
