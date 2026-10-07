package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	"parser/internal/api"
	"parser/internal/client"
	"parser/internal/dto"
	"parser/internal/logic"
	"parser/internal/repository"
	"parser/internal/search"

	"github.com/loviiin/project-argus/pkg/config"
	"github.com/loviiin/project-argus/pkg/dedup"
	"github.com/loviiin/project-argus/pkg/healthcheck"
	"github.com/loviiin/project-argus/pkg/metrics"
	"github.com/loviiin/project-argus/pkg/natsutil"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.LoadConfig()

	// Inicia Pprof em background
	go func() {
		slog.Info("Iniciando Pprof do Parser", "porta", ":6060")
		if err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
			slog.Error("Pprof server falhou", "error", err)
		}
	}()

	var repo repository.Repository
	var err error
	var indexer search.SearchIndexer

	var apiServer *api.Server

	if cfg.Database.Type == "sqlite" {
		sqliteRepo, errDB := repository.NewSQLiteRepository(cfg.Database.SQLitePath)
		if errDB != nil {
			slog.Error("Erro fatal no SQLite", "error", errDB)
			os.Exit(1)
		}
		repo = sqliteRepo
		indexer = search.NewSQLiteFTS5Indexer(sqliteRepo.DB())

		apiServer = api.NewServer(sqliteRepo.DB(), nil)
		go func() {
			if err := apiServer.Start(":8080"); err != nil {
				slog.Error("Erro no servidor da API", "error", err)
			}
		}()
	} else {
		pgRepo, errDB := repository.NewPostgresRepository(cfg.Database.URL)
		if errDB != nil {
			slog.Error("Erro fatal no Postgres", "error", errDB)
			os.Exit(1)
		}
		repo = pgRepo
		indexer = search.NewIndexer(cfg.Meilisearch.Host, cfg.Meilisearch.Key, cfg.Meilisearch.Index)
	}
	defer repo.Close(context.Background())

	supabaseClient := client.NewSupabaseClient(cfg.Supabase.URL, cfg.Supabase.Key)

	nc, err := nats.Connect(cfg.Nats.URL)
	if err != nil {
		slog.Error("Erro conectando ao NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	if apiServer != nil {
		apiServer.SetNATS(nc)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("Erro fatal: Redis não responde", "address", cfg.Redis.Address, "error", err)
		os.Exit(1)
	}

	dedupSv := dedup.NewDeduplicator(rdb, cfg.Redis.TTLHours)
	defer dedupSv.Close()

	js, _ := nc.JetStream()

	// Garantir que o stream de enrich exista
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "ENRICH",
		Subjects: []string{"jobs.enrich.>"},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		slog.Warn("Stream ENRICH check", "error", err)
	}

	// Garantir que o stream de extração de texto exista
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "TEXT_EXTRACTED",
		Subjects: []string{"data.text_extracted"},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		slog.Warn("Stream TEXT_EXTRACTED check", "error", err)
	}

	slog.Info("Parser Service Iniciado. Rodando Fast Ingestion Flow & Discord Enricher Flow...")

	parserMetrics := []metrics.MetricDef{
		{RedisKey: "argus:metrics:parser:processed", PromName: "argus_parser_processed_total", Help: "Total de mensagens processadas", Type: "counter"},
		{RedisKey: "argus:metrics:parser:invites_found", PromName: "argus_parser_invites_found_total", Help: "Total de convites Discord encontrados", Type: "counter"},
		{RedisKey: "argus:metrics:parser:enriched", PromName: "argus_parser_enriched_total", Help: "Total de servidores enriquecidos", Type: "counter"},
		{RedisKey: "argus:metrics:parser:errors", PromName: "argus_parser_errors_total", Help: "Total de errors", Type: "counter"},
	}
	healthHandler := healthcheck.New(nc, rdb, nil).Handler
	go metrics.StartMetricsServer(":8084", rdb, parserMetrics, healthHandler)

	finder := logic.NewDiscordFinder()
	discordClient := client.NewDiscordClient(cfg.Discord.ProxyURL, cfg.Discord.Token, cfg.Discord.FetchMode, rdb)

	// ==========================================
	// 1. FAST INGESTION FLOW
	// ==========================================
	subFast, err := js.Subscribe("data.text_extracted", natsutil.SafeHandler(func(msg *nats.Msg) {
		meta, err := msg.Metadata()
		if err != nil {
			msg.Ack()
			return
		}

		var payload dto.OcrMessage
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			slog.Error("Erro decodificando JSON [Fast Ingestion]", "error", err)
			rdb.Incr(context.Background(), "argus:metrics:parser:errors")
			msg.Ack()
			return
		}

		// Idempotência Determinística
		cleanPath := strings.TrimSpace(payload.SourcePath)
		hash := md5.Sum([]byte(cleanPath))
		hashStr := hex.EncodeToString(hash[:])

		// 1. Worker Heartbeat/Processing Lock
		lockKey := fmt.Sprintf("argus:processing_lock:fast_ingestion:%s", hashStr)
		if locked, _ := dedupSv.RDB().SetNX(context.Background(), lockKey, "1", 10*time.Minute).Result(); !locked {
			delay := time.Duration(30+rand.Intn(30)) * time.Second
			slog.Warn("Job bloqueado por lock. Nak + Jitter [Fast Ingestion]", "job", hashStr, "delay", delay)
			msg.NakWithDelay(delay)
			return
		}
		defer dedupSv.RDB().Del(context.Background(), lockKey)

		processed, err := dedupSv.CheckIfProcessed(context.Background(), "processed_job", "fast_ingestion:"+hashStr)
		if err == nil && processed {
			slog.Info("Mensagem duplicada ignorada [Fast Ingestion]", "job", hashStr)
			msg.Ack()
			return
		}

		if meta.NumDelivered > 15 {
			slog.Warn("Max Retries atingido. Enviando para DLQ... [Fast Ingestion]", "job", hashStr)
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

				slog.Info("Encontrado [Fast Ingestion]", "invite_code", inviteCode)
				rdb.Incr(context.Background(), "argus:metrics:parser:invites_found")

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
					slog.Error("Erro BD [Fast Ingestion]", "error", err)
					rdb.Incr(context.Background(), "argus:metrics:parser:errors")
					delay := time.Duration(math.Pow(5, float64(meta.NumDelivered-1))) * 5 * time.Second
					msg.NakWithDelay(delay)
					return
				}

				// Webhook Supabase assíncrono
				go func(invCode, pSourceUrl, pRawOcr, pAuthorId string, pIsReply bool) {
					if err := supabaseClient.SendArtifact(invCode, "", pSourceUrl, pRawOcr, pAuthorId, pIsReply); err != nil {
						slog.Error("Erro ao enviar para Supabase [Webhook]", "error", err)
					}
				}(inviteCode, payload.SourcePath, payload.TextContent, author, payload.IsReply)

				loc, _ := time.LoadLocation("America/Sao_Paulo")
				nowSP := time.Now().In(loc)

				meiliPayload := map[string]interface{}{
					"invite_code":         inviteCode,
					"invite_link":         "https://discord.gg/" + inviteCode,
					"source_url":          payload.SourcePath,
					"timestamp_formatted": nowSP.Format("02/01/2006 15:04:05"),
					"status":              "pending",
					"is_reply":            payload.IsReply,
				}
				if payload.ParentCommentID != "" {
					meiliPayload["parent_comment_id"] = payload.ParentCommentID
				}
				if payload.IsReply {
					meiliPayload["tags"] = []string{"[Evasion_Tactic]"}
				}

				err = indexer.IndexData(meiliPayload)
				if err != nil {
					slog.Error("Falha na indexação bruta [Fast Ingestion]", "error", err)
					rdb.Incr(context.Background(), "argus:metrics:parser:errors")
					delay := time.Duration(math.Pow(5, float64(meta.NumDelivered-1))) * 5 * time.Second
					msg.NakWithDelay(delay)
					return
				}

				enrichJob, _ := json.Marshal(dto.DiscordEnrichJob{InviteCode: inviteCode})
				if _, err := js.Publish("jobs.enrich.discord", enrichJob); err != nil {
					slog.Error("Erro publicando para enrich [Fast Ingestion]", "invite_code", inviteCode, "error", err)
					// Ignore publish errors so we don't block the ingestion flow fully
				}
			}
		}

		// Sucesso: Grava chave idempotencia e Ack
		dedupSv.MarkAsSeen(context.Background(), "processed_job", "fast_ingestion:"+hashStr)
		rdb.Incr(context.Background(), "argus:metrics:parser:processed")
		msg.Ack()
	}), nats.Durable("parser-fast-ingestion"), nats.DeliverAll(), nats.InactiveThreshold(30*time.Second), nats.ManualAck())

	if err != nil {
		slog.Error("Erro ao iniciar Fast Ingestion", "error", err)
		os.Exit(1)
	}

	// ==========================================
	// 2. DISCORD ENRICHER FLOW
	// ==========================================
	subEnrich, err := js.Subscribe("jobs.enrich.discord", natsutil.SafeHandler(func(msg *nats.Msg) {
		meta, err := msg.Metadata()
		if err != nil {
			msg.Ack()
			return
		}

		var job dto.DiscordEnrichJob
		if err := json.Unmarshal(msg.Data, &job); err != nil {
			slog.Error("Erro decodificando Job [Enricher]", "error", err)
			rdb.Incr(context.Background(), "argus:metrics:parser:errors")
			msg.Ack()
			return
		}

		// 1. Worker Heartbeat/Processing Lock
		lockKey := fmt.Sprintf("argus:processing_lock:%s", job.InviteCode)
		if locked, _ := dedupSv.RDB().SetNX(context.Background(), lockKey, "1", 10*time.Minute).Result(); !locked {
			delay := time.Duration(30+rand.Intn(30)) * time.Second
			slog.Warn("Job bloqueado por lock. Nak + Jitter [Enricher]", "invite_code", job.InviteCode, "delay", delay)
			msg.NakWithDelay(delay)
			return
		}
		defer dedupSv.RDB().Del(context.Background(), lockKey)

		processed, err := dedupSv.CheckIfProcessed(context.Background(), "processed_job", job.InviteCode)
		if err == nil && processed {
			slog.Info("Mensagem duplicada ignorada [Enricher]", "invite_code", job.InviteCode)
			msg.Ack()
			return
		}

		if meta.NumDelivered > 15 {
			slog.Warn("Max Retries atingido. Marcando como rate_limited e enviando para DLQ... [Enricher]", "invite_code", job.InviteCode)
			repo.UpdateStatus(context.Background(), job.InviteCode, "rate_limited")
			dlqData, _ := json.Marshal(map[string]interface{}{
				"error":    "Max retries exceeded",
				"job":      job,
				"metadata": map[string]interface{}{"num_delivered": meta.NumDelivered, "timestamp": time.Now()},
			})
			js.Publish("argus.dlq.parser_enricher", dlqData)
			msg.Ack()
			return
		}

		slog.Info("Processando [Enricher]", "invite_code", job.InviteCode, "tentativa", meta.NumDelivered)

		// 1. Checa no Meilisearch SE o registro já NÃO tem os campos enriquecidos:
		if existingDoc, err := indexer.GetDocument(job.InviteCode); err == nil {
			if existingDoc.ServerName != "" && existingDoc.Icon != "" {
				slog.Info("Skiped: já enriquecido. Poupando a API [Enricher]", "invite_code", job.InviteCode, "server_name", existingDoc.ServerName)
				dedupSv.MarkAsSeen(context.Background(), "processed_job", job.InviteCode)
				msg.Ack()
				return
			}
		}

		inviteInfo, err := discordClient.GetInviteInfo(context.Background(), job.InviteCode)
		if err != nil {
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "rate limited") || strings.Contains(errMsg, "429") || strings.Contains(errMsg, "circuit breaker") {
				slog.Warn("Circuit Breaker / Rate limit ativo. Estacionando como rate_limited [Enricher]", "invite_code", job.InviteCode)
				repo.UpdateStatus(context.Background(), job.InviteCode, "rate_limited")
				// NÃO marca como seen — o recovery goroutine vai retentar quando o circuit breaker liberar
				msg.Ack()
				return
			}
			if strings.Contains(errMsg, "inválido ou expirado") || strings.Contains(errMsg, "404") {
				slog.Info("Expirado. Marcando como 'expired' nas bases [Enricher]", "invite_code", job.InviteCode)

				// Atualizar registro como expirado
				indexer.UpdateData(map[string]interface{}{
					"invite_code": job.InviteCode,
					"status":      "expired",
				})
				repo.UpdateStatus(context.Background(), job.InviteCode, "expired")

				dedupSv.MarkAsSeen(context.Background(), "processed_job", job.InviteCode)
				msg.Ack()
				return
			}

			// Outros erros
			slog.Error("Erro inesperado [Enricher]", "invite_code", job.InviteCode, "error", err)
			rdb.Incr(context.Background(), "argus:metrics:parser:errors")
			delay := time.Duration(math.Pow(5, float64(meta.NumDelivered-1))) * 5 * time.Second
			msg.NakWithDelay(delay)
			return
		}

		slog.Info("Dados Sucesso [Enricher]", "invite_code", job.InviteCode, "server_name", inviteInfo.Guild.Name, "membros", inviteInfo.ApproximateMemberCount)
		rdb.Incr(context.Background(), "argus:metrics:parser:enriched")

		var iconURL string
		if inviteInfo.Guild.Icon != "" {
			if strings.HasPrefix(inviteInfo.Guild.Icon, "https://") {
				// Modo scraper: rod_client já retorna a URL completa do og:image
				iconURL = inviteInfo.Guild.Icon
			} else {
				// Modo API: icon é apenas o hash, monta a URL
				ext := "png"
				if strings.HasPrefix(inviteInfo.Guild.Icon, "a_") {
					ext = "gif"
				}
				iconURL = fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.%s", inviteInfo.Guild.ID, inviteInfo.Guild.Icon, ext)
			}
		}

		err = indexer.UpdateData(map[string]interface{}{
			"invite_code":  job.InviteCode,
			"invite_link":  "https://discord.gg/" + job.InviteCode,
			"server_name":  inviteInfo.Guild.Name,
			"image":        iconURL,
			"member_count": inviteInfo.ApproximateMemberCount,
			"status":       "active",
		})
		if err != nil {
			slog.Error("Erro ao atualizar Meilisearch [Enricher]", "error", err)
			rdb.Incr(context.Background(), "argus:metrics:parser:errors")
			delay := time.Duration(math.Pow(5, float64(meta.NumDelivered-1))) * 5 * time.Second
			msg.NakWithDelay(delay)
			return
		}

		if err := repo.UpdateEnrichedData(context.Background(), job.InviteCode, inviteInfo.Guild.Name, inviteInfo.Guild.ID, iconURL, inviteInfo.ApproximateMemberCount, "active"); err != nil {
			slog.Error("Erro ao atualizar PostgreSQL [Enricher]", "error", err)
			rdb.Incr(context.Background(), "argus:metrics:parser:errors")
			delay := time.Duration(math.Pow(5, float64(meta.NumDelivered-1))) * 5 * time.Second
			msg.NakWithDelay(delay)
			return
		}

		// Auto tag
		tags := logic.AutoTag(inviteInfo.Guild.Name, "")
		if tags != "" {
			_ = repo.UpdateTags(context.Background(), job.InviteCode, tags)
		}

		dedupSv.MarkAsSeen(context.Background(), "processed_job", job.InviteCode)
		rdb.Incr(context.Background(), "argus:metrics:parser:processed")
		msg.Ack()
	}), nats.Durable("discord-enricher"), nats.DeliverAll(), nats.ManualAck())

	if err != nil {
		slog.Error("Erro ao iniciar Discord Enricher", "error", err)
		os.Exit(1)
	}

	// ==========================================
	// 3. RECOVERY: Re-enrich rate_limited invites
	// ==========================================
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			circuitKey := "argus:circuit_breaker:discord"
			if exists, _ := rdb.Exists(context.Background(), circuitKey).Result(); exists > 0 {
				// Circuit breaker ainda ativo, não tenta nada
				continue
			}

			invites, err := repo.GetRateLimitedInvites(context.Background(), 200)
			if err != nil {
				slog.Error("Erro buscando invites rate_limited [Recovery]", "error", err)
				continue
			}

			if len(invites) == 0 {
				continue
			}

			slog.Info("Circuit breaker livre. Re-publicando invites rate_limited... [Recovery]", "quantidade", len(invites))
			for _, code := range invites {
				// Limpa a flag de dedup pra esse invite poder ser processado
				dedupSv.RDB().Del(context.Background(), fmt.Sprintf("argus:processed_job:%s", code))

				enrichJob, _ := json.Marshal(dto.DiscordEnrichJob{InviteCode: code})
				if _, err := js.Publish("jobs.enrich.discord", enrichJob); err != nil {
					slog.Error("Erro re-publicando [Recovery]", "invite_code", code, "error", err)
				}
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	slog.Info("Sinal recebido. Drenando conexões NATS para Graceful Shutdown...")

	err = subFast.Drain()
	if err != nil {
		slog.Error("Erro ao drenar Fast Ingestion", "error", err)
	}

	err = subEnrich.Drain()
	if err != nil {
		slog.Error("Erro ao drenar Discord Enricher", "error", err)
	}

	// Drain é assíncrono ou síncrono dependendo do uso; nas versões recentes Wait() é necessário ou Time Sleep de garantia
	// Mas como nc.Close() também aguarda/interrompe o resto, isso é suficiente.
	time.Sleep(1 * time.Second)
	slog.Info("Parser Service encerrado gracefully.")
}

