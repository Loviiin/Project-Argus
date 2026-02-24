package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
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
		meta, err := msg.Metadata()
		if err != nil {
			msg.Ack()
			return
		}

		var payload dto.OcrMessage
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			log.Printf("[Fast Ingestion] Erro decodificando JSON: %v", err)
			msg.Ack()
			return
		}

		// Idempotência Determinística
		cleanPath := strings.TrimSpace(payload.SourcePath)
		hash := md5.Sum([]byte(cleanPath))
		hashStr := hex.EncodeToString(hash[:])
		idempotencyKey := fmt.Sprintf("argus:processed_job:fast_ingestion:%s", hashStr)

		exists, err := rdb.Exists(context.Background(), idempotencyKey).Result()
		if err == nil && exists > 0 {
			log.Printf("[Fast Ingestion] Mensagem duplicada ignorada: %s", hashStr)
			msg.Ack()
			return
		}

		if meta.NumDelivered > 5 {
			log.Printf("[Fast Ingestion] 🚨 Max Retries atingido para %s. Enviando para DLQ...", hashStr)
			dlqData, _ := json.Marshal(map[string]interface{}{
				"error":    "Max retries exceeded",
				"payload":  payload,
				"metadata": map[string]interface{}{"num_delivered": meta.NumDelivered, "timestamp": time.Now()},
			})
			js.Publish("argus.dlq.parser_fast", dlqData)
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
					DiscordStatus:     "pending",
				}

				if _, err := repo.Save(context.Background(), artifact); err != nil {
					fmt.Printf("[Fast Ingestion] Erro BD: %v\n", err)
					delay := time.Duration(math.Pow(5, float64(meta.NumDelivered-1))) * 5 * time.Second
					msg.NakWithDelay(delay)
					return
				}

				err = indexer.IndexData(map[string]interface{}{
					"invite_code":         inviteCode,
					"source_url":          payload.SourcePath,
					"timestamp_formatted": time.Now().Format("02/01/2006 15:04:05"),
					"status":              "pending",
				})
				if err != nil {
					fmt.Printf("[Fast Ingestion] Falha na indexação bruta: %v\n", err)
					delay := time.Duration(math.Pow(5, float64(meta.NumDelivered-1))) * 5 * time.Second
					msg.NakWithDelay(delay)
					return
				}

				enrichJob, _ := json.Marshal(dto.DiscordEnrichJob{InviteCode: inviteCode})
				if _, err := js.Publish("jobs.enrich.discord", enrichJob); err != nil {
					log.Printf("[Fast Ingestion] Erro publicando %s para enrich: %v", inviteCode, err)
					// Ignore publish errors so we don't block the ingestion flow fully
				}
			}
		}

		// Sucesso: Grava chave idempotencia e Ack
		rdb.Set(context.Background(), idempotencyKey, "1", 7*24*60*60*time.Second)
		msg.Ack()
	}, nats.Durable("parser-fast-ingestion"), nats.DeliverAll(), nats.InactiveThreshold(30*time.Second), nats.ManualAck())

	if err != nil {
		log.Fatalf("Erro ao iniciar Fast Ingestion: %v", err)
	}

	// ==========================================
	// 2. DISCORD ENRICHER FLOW
	// ==========================================
	subEnrich, err := js.Subscribe("jobs.enrich.discord", func(msg *nats.Msg) {
		meta, err := msg.Metadata()
		if err != nil {
			msg.Ack()
			return
		}

		var job dto.DiscordEnrichJob
		if err := json.Unmarshal(msg.Data, &job); err != nil {
			log.Printf("[Enricher] Erro decodificando Job: %v", err)
			msg.Ack()
			return
		}

		idempotencyKey := fmt.Sprintf("argus:processed_job:%s", job.InviteCode)
		exists, err := rdb.Exists(context.Background(), idempotencyKey).Result()
		if err == nil && exists > 0 {
			log.Printf("[Enricher] Mensagem duplicada ignorada: %s", job.InviteCode)
			msg.Ack()
			return
		}

		if meta.NumDelivered > 5 {
			log.Printf("[Enricher] 🚨 Max Retries atingido para %s. Enviando para DLQ...", job.InviteCode)
			dlqData, _ := json.Marshal(map[string]interface{}{
				"error":    "Max retries exceeded",
				"job":      job,
				"metadata": map[string]interface{}{"num_delivered": meta.NumDelivered, "timestamp": time.Now()},
			})
			js.Publish("argus.dlq.parser_enricher", dlqData)
			msg.Ack()
			return
		}

		fmt.Printf("[Enricher] Processando: %s [Tentativa: %d]\n", job.InviteCode, meta.NumDelivered)

		// 1. Checa no Meilisearch SE o registro já NÃO tem os campos enriquecidos:
		if existingDoc, err := indexer.GetDocument(job.InviteCode); err == nil {
			if existingDoc.ServerName != "" && existingDoc.Icon != "" {
				fmt.Printf("[Enricher] ⏭️ Skiped: %s já enriquecido (%s). Poupando a API.\n", job.InviteCode, existingDoc.ServerName)
				rdb.Set(context.Background(), idempotencyKey, "1", 7*24*60*60*time.Second)
				msg.Ack()
				return
			}
		}

		inviteInfo, err := discordClient.GetInviteInfo(context.Background(), job.InviteCode)
		if err != nil {
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "rate limited") || strings.Contains(errMsg, "429") {
				fmt.Printf("[Enricher] Rate limited no Discord API para %s. Nak 1 min.\n", job.InviteCode)
				msg.NakWithDelay(1 * time.Minute)
				return
			}
			if strings.Contains(errMsg, "inválido ou expirado") || strings.Contains(errMsg, "404") {
				fmt.Printf("[Enricher] %s expirado. Marcando como 'expired' nas bases.\n", job.InviteCode)

				// Atualizar registro como expirado
				indexer.UpdateData(map[string]interface{}{
					"invite_code": job.InviteCode,
					"status":      "expired",
				})
				repo.UpdateEnrichedData(context.Background(), job.InviteCode, "", "", "", 0, "expired")

				rdb.Set(context.Background(), idempotencyKey, "1", 7*24*60*60*time.Second)
				msg.Ack()
				return
			}

			// Outros erros
			fmt.Printf("[Enricher] Erro inesperado %s: %v\n", job.InviteCode, err)
			delay := time.Duration(math.Pow(5, float64(meta.NumDelivered-1))) * 5 * time.Second
			msg.NakWithDelay(delay)
			return
		}

		fmt.Printf("[Enricher] Dados Sucesso: %s → %s | Membros: %d\n",
			job.InviteCode, inviteInfo.Guild.Name, inviteInfo.ApproximateMemberCount)

		var iconURL string
		if inviteInfo.Guild.Icon != "" {
			iconURL = fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.png", inviteInfo.Guild.ID, inviteInfo.Guild.Icon)
		}

		err = indexer.UpdateData(map[string]interface{}{
			"invite_code":  job.InviteCode,
			"server_name":  inviteInfo.Guild.Name,
			"icon":         iconURL,
			"member_count": inviteInfo.ApproximateMemberCount,
			"status":       "active",
		})
		if err != nil {
			fmt.Printf("[Enricher] Erro ao atualizar Meilisearch: %v\n", err)
			delay := time.Duration(math.Pow(5, float64(meta.NumDelivered-1))) * 5 * time.Second
			msg.NakWithDelay(delay)
			return
		}

		if err := repo.UpdateEnrichedData(context.Background(), job.InviteCode, inviteInfo.Guild.Name, inviteInfo.Guild.ID, iconURL, inviteInfo.ApproximateMemberCount, "active"); err != nil {
			fmt.Printf("[Enricher] Erro ao atualizar PostgreSQL: %v\n", err)
			delay := time.Duration(math.Pow(5, float64(meta.NumDelivered-1))) * 5 * time.Second
			msg.NakWithDelay(delay)
			return
		}

		rdb.Set(context.Background(), idempotencyKey, "1", 7*24*60*60*time.Second)
		msg.Ack()
	}, nats.Durable("discord-enricher"), nats.DeliverAll(), nats.ManualAck())

	if err != nil {
		log.Fatalf("Erro ao iniciar Discord Enricher: %v", err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("\nSinal recebido. Drenando conexões NATS para Graceful Shutdown...")

	err = subFast.Drain()
	if err != nil {
		fmt.Printf("Erro ao drenar Fast Ingestion: %v\n", err)
	}

	err = subEnrich.Drain()
	if err != nil {
		fmt.Printf("Erro ao drenar Discord Enricher: %v\n", err)
	}

	// Drain é assíncrono ou síncrono dependendo do uso; nas versões recentes Wait() é necessário ou Time Sleep de garantia
	// Mas como nc.Close() também aguarda/interrompe o resto, isso é suficiente.
	time.Sleep(1 * time.Second)
	fmt.Println("Parser Service encerrado gracefully.")
}
