package config

import (
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config representa a estrutura completa do config.yaml
type Config struct {
	App struct {
		Env string `yaml:"env"`
	} `yaml:"app"`

	// Usado pelo Discovery
	Discovery struct {
		Hashtags []string `yaml:"hashtags"`
		Interval int      `yaml:"interval_seconds"`
	} `yaml:"discovery"`

	// Usado pelo Parser (Legacy) ou Discovery
	Targets struct {
		Hashtags []string `yaml:"hashtags"`
	} `yaml:"targets"`

	// Infraestrutura Compartilhada
	Nats struct {
		URL string `yaml:"url"`
	} `yaml:"nats"`

	Redis struct {
		Address  string `yaml:"address"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"redis"`

	// Específico do Parser
	Database struct {
		URL string `yaml:"url"`
	} `yaml:"database"`

	Meilisearch struct {
		Host  string `yaml:"host"`
		Key   string `yaml:"key"`
		Index string `yaml:"index"`
	} `yaml:"meilisearch"`

	Discord struct {
		Token string `yaml:"token"`
	} `yaml:"discord"`
}

func LoadConfig() *Config {
	// 1. Tenta pegar via Variável de Ambiente (Docker/Prod)
	configPath := os.Getenv("CONFIG_PATH")

	// 2. Se não tiver, tenta achar "subindo" pastas (Local Dev)
	if configPath == "" {
		// Tenta no diretório atual
		if _, err := os.Stat("config.yaml"); err == nil {
			configPath = "config.yaml"
		} else if _, err := os.Stat("config/config.yaml"); err == nil {
			configPath = "config/config.yaml"
		} else if _, err := os.Stat("../../config/config.yaml"); err == nil {
			// Fallback agressivo: Tenta subir até achar a raiz
			// Útil quando rodamos 'go run' de dentro de cmd/
			configPath = "../../config/config.yaml"
		}
	}

	// Converte caminho relativo para absoluto para debug
	absPath, _ := filepath.Abs(configPath)
	log.Printf("Carregando config de: %s", absPath)

	f, err := os.Open(configPath)
	if err != nil {
		// Última tentativa: hardcoded para devcontainer se falhar
		f, err = os.Open("/workspaces/Project-Argus/config/config.yaml")
		if err != nil {
			log.Fatalf("Erro fatal lendo config: %v", err)
		}
	}
	defer f.Close()

	var cfg Config
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		log.Fatalf("Erro ao decodificar YAML: %v", err)
	}

	return &cfg
}
