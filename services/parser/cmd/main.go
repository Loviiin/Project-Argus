package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	"parser/internal/client"
	"parser/internal/dto"
	"parser/internal/logic"
	"parser/internal/repository"
	"parser/internal/search"

	"github.com/loviiin/project-argus/pkg/config"
)

func main() {
	cfg := config.LoadConfig()

	repo, err := repository.NewArtifactRepository(cfg.Database.URL)
	if err != nil {
		log.Fatal("Erro fatal no banco:", err)
	}
	defer repo.Close(context.Background())

	indexer := search.NewIndexer(cfg.Meilisearch.Host, cfg.Meilisearch.Key, cfg.Meilisearch.Index)

	nc, err := nats.Connect(cfg.Nats.URL)
	if err != nil {
		log.Fatal("Erro conectando ao NATS:", err)
	}
	defer nc.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Erro fatal: Redis não responde em %s: %v", cfg.Redis.Address, err)
	}

	js, _ := nc.JetStream()
	fmt.Println("Parser Service (Go) iniciado. Aguardando textos...")

	finder := logic.NewDiscordFinder()
	discordClient := client.NewDiscordClient(cfg.Discord.Token, rdb)
	sub, err := js.Subscribe("data.text_extracted", func(msg *nats.Msg) {

		var payload dto.OcrMessage
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			log.Printf("Erro decodificando JSON: %v", err)
			msg.Ack()
			return
		}

		fmt.Printf("Analisando texto de: %s\n", payload.SourcePath)

		invites := finder.FindInvites(payload.TextContent)

		if len(invites) > 0 {
			fmt.Printf("Convites encontrados: %v\n", invites)

			for _, inviteLink := range invites {
				var inviteCode string
				if strings.HasPrefix(inviteLink, "discord.gg/") {
					inviteCode = strings.TrimPrefix(inviteLink, "discord.gg/")
				} else if strings.HasPrefix(inviteLink, "discord.com/invite/") {
					inviteCode = strings.TrimPrefix(inviteLink, "discord.com/invite/")
				} else {
					continue
				}

				inviteInfo, err := discordClient.GetInviteInfo(context.Background(), inviteCode)
				if err != nil {
					fmt.Printf("Erro consultando %s: %v\n", inviteLink, err)
					continue
				}

				fmt.Printf("%s → Guild: %s (ID: %s) | Membros: ~%d\n",
					inviteLink, inviteInfo.Guild.Name, inviteInfo.Guild.ID, inviteInfo.ApproximateMemberCount)

				fmt.Printf("Salvando '%s' no banco...\n", inviteInfo.Guild.Name)

				author := payload.AuthorID
				if author == "" {
					author = "desconhecido"
				}

				artifact := repository.Artifact{
					SourceURL:          payload.SourcePath,
					AuthorID:           author,
					DiscordInviteCode:  inviteCode,
					DiscordServerName:  inviteInfo.Guild.Name,
					DiscordServerID:    inviteInfo.Guild.ID,
					DiscordMemberCount: inviteInfo.ApproximateMemberCount,
					RawOcrText:         payload.TextContent,
					RiskScore:          0,
				}

				artifactID, err := repo.Save(context.Background(), artifact)
				if err != nil {
					fmt.Printf("Erro ao salvar no DB: %v\n", err)
				} else {
					fmt.Printf("Salvo com sucesso (ID: %s). Indexando...\n", artifactID)

					var iconURL string
					if inviteInfo.Guild.Icon != "" {
						iconURL = fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.png", inviteInfo.Guild.ID, inviteInfo.Guild.Icon)
					}

					err := indexer.IndexData(search.SearchDoc{
						ID:         artifactID,
						ServerName: artifact.DiscordServerName,
						InviteCode: artifact.DiscordInviteCode,
						SourceURL:  artifact.SourceURL,
						Timestamp:  time.Now().Unix(),
						Icon:       iconURL,
					})

					if err != nil {
						fmt.Printf("Falha na indexação: %v\n", err)
					}
				}
			}
		} else {
			fmt.Printf("Nenhum convite encontrado.\n")
		}

		msg.Ack()
	}, nats.Durable("parser-consumer"), nats.DeliverAll(), nats.InactiveThreshold(30*time.Second))

	if err != nil {
		log.Fatal(err)
	}

	fmt.Print(sub)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
