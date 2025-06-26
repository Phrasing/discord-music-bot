package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	BotToken    string
	CookiesPath string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables")
	}

	return &Config{
		BotToken:    os.Getenv("BOT_TOKEN"),
		CookiesPath: os.Getenv("COOKIES_PATH"),
	}
}
