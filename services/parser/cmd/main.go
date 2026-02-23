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

	// Garantir que o stream de enrich exista
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "ENRICH",
		Subjects: []string{"jobs.enrich.>"},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		fmt.Printf("Stream ENRICH check: %v\n", err)
	}

	fmt.Println("Parser Service Iniciado. Rodando Fast Ingestion Flow & Discord Enricher Flow...")

	finder := logic.NewDiscordFinder()
	discordClient := client.NewDiscordClient(cfg.Discord.FetchMode, cfg.Discord.ProxyURL, cfg.Discord.Token, rdb)

	// ==========================================
	// 1. FAST INGESTION FLOW
	// ==========================================
	subFast, err := js.Subscribe("data.text_extracted", func(msg *nats.Msg) {
		var payload dto.OcrMessage
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			log.Printf("[Fast Ingestion] Erro decodificando JSON: %v", err)
			msg.Ack()
			return
		}

		invites := finder.FindInvites(payload.TextContent)
		if len(invites) > 0 {
			// Dedup em memória rápida para não mandar enriquecer 2x o mesmo link no mesmo video
			seen := make(map[string]bool)

			for _, inviteLink := range invites {
				var inviteCode string
				if strings.HasPrefix(inviteLink, "discord.gg/") {
					inviteCode = strings.TrimPrefix(inviteLink, "discord.gg/")
				} else if strings.HasPrefix(inviteLink, "discord.com/invite/") {
					inviteCode = strings.TrimPrefix(inviteLink, "discord.com/invite/")
				} else {
					continue
				}

				if seen[inviteCode] {
					continue
				}
				seen[inviteCode] = true

				fmt.Printf("[Fast Ingestion] Encontrado: %s\n", inviteCode)

				// Salva bruto no banco (se falhar ignora para não travar o loop)
				author := payload.AuthorID
				if author == "" {
					author = "desconhecido"
				}

				artifact := repository.Artifact{
					SourceURL:         payload.SourcePath,
					AuthorID:          author,
					DiscordInviteCode: inviteCode,
					RawOcrText:        payload.TextContent,
					RiskScore:         0,
				}
				repo.Save(context.Background(), artifact)

				// Upsert imediato e BRUTO no Meilisearch
				err := indexer.IndexData(map[string]interface{}{
					"invite_code":         inviteCode,
					"source_url":          payload.SourcePath,
					"timestamp_formatted": time.Now().Format("02/01/2006 15:04:05"),
				})
				if err != nil {
					fmt.Printf("[Fast Ingestion] Falha na indexação bruta: %v\n", err)
				}

				// Publica no tópico de Enrich (Assíncrono)
				enrichJob, _ := json.Marshal(dto.DiscordEnrichJob{InviteCode: inviteCode})
				js.Publish("jobs.enrich.discord", enrichJob)
			}
		}

		msg.Ack()
	}, nats.Durable("parser-fast-ingestion"), nats.DeliverAll(), nats.InactiveThreshold(30*time.Second))

	if err != nil {
		log.Fatalf("Erro ao iniciar Fast Ingestion: %v", err)
	}
	defer subFast.Unsubscribe()

	// ==========================================
	// 2. DISCORD ENRICHER FLOW
	// ==========================================
	subEnrich, err := js.Subscribe("jobs.enrich.discord", func(msg *nats.Msg) {
		var job dto.DiscordEnrichJob
		if err := json.Unmarshal(msg.Data, &job); err != nil {
			log.Printf("[Enricher] Erro decodificando Job: %v", err)
			msg.Ack()
			return
		}

		fmt.Printf("[Enricher] Processando: %s\n", job.InviteCode)

		// 1. Checa no Meilisearch SE o registro já NÃO tem os campos enriquecidos:
		if existingDoc, err := indexer.GetDocument(job.InviteCode); err == nil {
			if existingDoc.ServerName != "" && existingDoc.Icon != "" {
				// Já foi enriquecido previamente! Podemos poupar a API do Discord e ignorar:
				fmt.Printf("[Enricher] ⏭️ Skiped: %s já enriquecido (%s). Poupando a API.\n", job.InviteCode, existingDoc.ServerName)
				msg.Ack()
				return
			}
		}

		// Chama a API do Discord (ou Redis cache).
		// Se bater Rate Limit (429), erro vai ser "rate limited".
		inviteInfo, err := discordClient.GetInviteInfo(context.Background(), job.InviteCode)
		if err != nil {
			if strings.Contains(err.Error(), "rate limited") {
				// Devolve pra fila com um delay maior pra mandar pro "final da fila"
				// O rate limit do Discord bloqueia o IP momentaneamente.
				msg.NakWithDelay(1 * time.Minute)
				return
			}
			if strings.Contains(err.Error(), "inválido ou expirado") {
				// Erro permanente, descarta
				fmt.Printf("[Enricher] %s expirado/inválido. Descartando.\n", job.InviteCode)
				msg.Ack()
				return
			}

			// Outros erros
			fmt.Printf("[Enricher] Erro inesperado %s: %v\n", job.InviteCode, err)
			msg.NakWithDelay(1 * time.Minute)
			return
		}

		fmt.Printf("[Enricher] Dados Sucesso: %s → %s | Membros: %d\n",
			job.InviteCode, inviteInfo.Guild.Name, inviteInfo.ApproximateMemberCount)

		// Parial Update no Meilisearch com os dados ricos
		var iconURL string
		if inviteInfo.Guild.Icon != "" {
			iconURL = fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.png", inviteInfo.Guild.ID, inviteInfo.Guild.Icon)
		}

		err = indexer.UpdateData(map[string]interface{}{
			"invite_code":  job.InviteCode,
			"server_name":  inviteInfo.Guild.Name,
			"icon":         iconURL,
			"member_count": inviteInfo.ApproximateMemberCount,
		})
		if err != nil {
			fmt.Printf("[Enricher] Erro ao atualizar Meilisearch: %v\n", err)
		}

		// Atualiza SOMENTE os dados enriquecidos no PostgreSQL (sem tocar nos dados existentes)
		if err := repo.UpdateEnrichedData(context.Background(), job.InviteCode, inviteInfo.Guild.Name, inviteInfo.Guild.ID, iconURL, inviteInfo.ApproximateMemberCount); err != nil {
			fmt.Printf("[Enricher] Erro ao atualizar PostgreSQL: %v\n", err)
		}

		msg.Ack()
	}, nats.Durable("discord-enricher"), nats.DeliverAll())

	if err != nil {
		log.Fatalf("Erro ao iniciar Discord Enricher: %v", err)
	}
	defer subEnrich.Unsubscribe()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
