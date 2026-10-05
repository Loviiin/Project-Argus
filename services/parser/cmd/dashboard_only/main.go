package main

import (
	"log/slog"
	"os"
	"parser/internal/api"
	"parser/internal/repository"
)

func main() {
	// Cria logger padrão
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Inicia conexão SQLite sem precisar do Redis ou NATS
	slog.Info("Conectando ao banco SQLite local...")
	sqliteRepo, err := repository.NewSQLiteRepository("../../data/argus.db")
	if err != nil {
		slog.Error("Falha ao abrir banco local", "err", err)
		os.Exit(1)
	}

	// Inicializa o servidor da API (que inclui o web dashboard estático)
	apiServer := api.NewServer(sqliteRepo.DB(), nil)

	slog.Info("Dashboard Standalone iniciado com sucesso! Acesse: http://localhost:8080")
	if err := apiServer.Start(":8080"); err != nil {
		slog.Error("Erro no servidor", "err", err)
	}
}
