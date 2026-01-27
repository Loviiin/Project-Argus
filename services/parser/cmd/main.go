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

	"parser/internal/client"
	"parser/internal/config"
	"parser/internal/dto"
	"parser/internal/logic"
	"parser/internal/repository"
	"parser/internal/search"
)

func main() {
	cfg := config.LoadConfig()
	fmt.Printf("Config carregada: env=%s, hashtags=%d, token=%t\n",
		cfg.App.Env, len(cfg.Targets.Hashtags), cfg.Discord.Token != "")

	repo, err := repository.NewArtifactRepository(cfg.Database.URL)
	if err != nil {
		log.Fatal("Erro fatal no banco:", err)
	}
	defer repo.Close(context.Background())
	fmt.Println("Conectado ao PostgreSQL!")

	indexer := search.NewIndexer(cfg.Meilisearch.Host, cfg.Meilisearch.Key, cfg.Meilisearch.Index)

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatal("Erro conectando ao NATS:", err)
	}
	defer nc.Close()

	js, _ := nc.JetStream()
	fmt.Println("Parser Service (Go) iniciado. Aguardando textos...")

	finder := logic.NewDiscordFinder()
	discordClient := client.NewDiscordClient(cfg.Discord.Token)

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

				inviteInfo, err := discordClient.GetInviteInfo(inviteCode)
				if err != nil {
					fmt.Printf("Erro consultando %s: %v\n", inviteLink, err)
					continue
				}

				fmt.Printf("%s → Guild: %s (ID: %s) | Membros: ~%d\n",
					inviteLink, inviteInfo.Guild.Name, inviteInfo.Guild.ID, inviteInfo.ApproximateMemberCount)

				fmt.Printf("Salvando '%s' no banco...\n", inviteInfo.Guild.Name)
				artifact := repository.Artifact{
					SourceURL:          payload.SourcePath,
					AuthorID:           "desconhecido_por_enquanto",
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

					err := indexer.IndexData(search.SearchDoc{
						ID:         artifactID,
						ServerName: artifact.DiscordServerName,
						InviteCode: artifact.DiscordInviteCode,
						SourceURL:  artifact.SourceURL,
						Timestamp:  time.Now().Unix(),
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

	// Mantém o container rodando até receber Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
