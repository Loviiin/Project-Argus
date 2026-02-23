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

	Targets struct {
		Hashtags []string `yaml:"hashtags"`
	} `yaml:"targets"`

	Nats struct {
		URL string `yaml:"url"`
	} `yaml:"nats"`

	Scraper struct {
		Workers         int    `yaml:"workers"`
		BrowserStateDir string `yaml:"browser_state_dir"`
	} `yaml:"scraper"`

	Redis struct {
		Address  string `yaml:"address"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"redis"`

	Database struct {
		URL string `yaml:"url"`
	} `yaml:"database"`

	Meilisearch struct {
		Host  string `yaml:"host"`
		Key   string `yaml:"key"`
		Index string `yaml:"index"`
	} `yaml:"meilisearch"`

	Discord struct {
		FetchMode string `yaml:"fetch_mode"`
		Token     string `yaml:"token"` // opcional
		ProxyURL  string `yaml:"proxy"` // opcional, formato: http://user:pass@ip:port
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
