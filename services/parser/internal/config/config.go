package config

import (
	"log"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	App struct {
		Env string `mapstructure:"env"`
	} `mapstructure:"app"`

	Discord struct {
		Token string `mapstructure:"token"`
	} `mapstructure:"discord"`

	Database struct {
		URL string `mapstructure:"url"`
	} `mapstructure:"database"`

	Targets struct {
		Hashtags []string `mapstructure:"hashtags"`
	} `mapstructure:"targets"`
}

func LoadConfig() *Config {
	var cfg Config

	viper.AddConfigPath("../../config")
	viper.AddConfigPath("/app/config")
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Aviso: Config não encontrada (%s). Usando padrões/env vars.", err)
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatal("Erro fatal no parse da config:", err)
	}

	return &cfg
}
