package tiktok

import (
	"context"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/loviiin/project-argus/pkg/dedup"
	"github.com/redis/go-redis/v9"
)

func TestDeduplicationFix(t *testing.T) {
	// Setup MiniRedis pra rodar os testes sem precisar do Redis real subindo
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Erro ao iniciar miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	dedupSvc := dedup.NewDeduplicator(rdb, 24)
	ctx := context.Background()

	// Simulando o Scraper (Subscriber) salvando um vídeo que ele terminou de processar (prefixo processed_job)
	videoID := "7610549811661540615"
	err = dedupSvc.MarkAsSeen(ctx, "processed_job", videoID)
	if err != nil {
		t.Fatalf("Erro no mock save: %v", err)
	}

	// 1: Testando a consulta ANTIGA que o Discovery fazia (usava "seen") - Deve retornar que o vídeo NÃO foi visto
	isProcessedOldWay, _ := dedupSvc.CheckIfProcessed(ctx, "seen", videoID)
	if isProcessedOldWay {
		t.Errorf("A forma antiga de consultar ('seen') retornou true, mas devia retornar false porque o prefixo está errado")
	}

	// 2: Testando a consulta NOVA que acabamos de colocar no código do Discovery ("processed_job") - Deve retornar true!
	isProcessedNewWay, _ := dedupSvc.CheckIfProcessed(ctx, "processed_job", videoID)
	if !isProcessedNewWay {
		t.Errorf("A forma nova/corrigida ('processed_job') retornou false, mas devia identificar que o vídeo já foi processado pelo Scraper")
	}
}
