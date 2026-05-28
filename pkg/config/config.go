package config

import (
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App struct {
		Env string `yaml:"env"`
	} `yaml:"app"`

	Discovery struct {
		Hashtags []string `yaml:"hashtags"`
		Interval int      `yaml:"interval_seconds"`
		Workers  int      `yaml:"workers"`
	} `yaml:"discovery"`

	TikTok struct {
		// Cookie de sessão anónima. Obter em tiktok.com > F12 > Cookies > ttwid.
		// Dura ~30-60 dias. Não requer conta TikTok.
		Ttwid string `yaml:"ttwid"`
		// URL base do sidecar
		SidecarURL string `yaml:"sidecar_url"`
	} `yaml:"tiktok"`

	Nats struct {
		URL string `yaml:"url"`
	} `yaml:"nats"`

	Scraper struct {
		Workers      int `yaml:"workers"`
		ReplyWorkers int `yaml:"reply_workers"`
	} `yaml:"scraper"`

	Supabase struct {
		URL string `yaml:"url"`
		Key string `yaml:"key"`
	} `yaml:"supabase"`

	Redis struct {
		Address  string `yaml:"address"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
		TTLHours int    `yaml:"ttl_hours"`
	} `yaml:"redis"`

	Database struct {
		Type       string `yaml:"type"` // "postgres" ou "sqlite"
		URL        string `yaml:"url"`
		SQLitePath string `yaml:"sqlite_path"` // ex: "./data/argus.db"
	} `yaml:"database"`

	Meilisearch struct {
		Host  string `yaml:"host"`
		Key   string `yaml:"key"`
		Index string `yaml:"index"`
	} `yaml:"meilisearch"`

	Discord struct {
		Token    string `yaml:"token"` // opcional
		ProxyURL string `yaml:"proxy"` // opcional, formato: http://user:pass@ip:port
	} `yaml:"discord"`
}

func LoadConfig() *Config {
	configPath := os.Getenv("CONFIG_PATH")

	if configPath == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			configPath = "config.yaml"
		} else if _, err := os.Stat("config/config.yaml"); err == nil {
			configPath = "config/config.yaml"
		} else if _, err := os.Stat("../../config/config.yaml"); err == nil {
			configPath = "../../config/config.yaml"
		}
	}

	absPath, _ := filepath.Abs(configPath)
	log.Printf("Loading config from: %s", absPath)

	f, err := os.Open(configPath)
	if err != nil {
		f, err = os.Open("/workspaces/Project-Argus/config/config.yaml")
		if err != nil {
			log.Fatalf("Fatal: could not read config: %v", err)
		}
	}
	defer f.Close()

	var cfg Config
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		log.Fatalf("Error decoding YAML: %v", err)
	}

	return &cfg
}
