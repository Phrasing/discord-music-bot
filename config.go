package main

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	BotToken            string
	CookiesPath         string
	SpotifyClientID     string
	SpotifyClientSecret string
	YtDlpProxy          string
	OpusBitrate         int
	OpusComplexity      int
	OpusInBandFEC       bool
	OpusPacketLossPerc  int
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables")
	}

	return &Config{
		BotToken:            os.Getenv("BOT_TOKEN"),
		CookiesPath:         os.Getenv("COOKIES_PATH"),
		SpotifyClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		SpotifyClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		YtDlpProxy:          os.Getenv("YT_DLP_PROXY"),
		OpusBitrate:         getEnvAsInt("OPUS_BITRATE", 128000),
		OpusComplexity:      getEnvAsInt("OPUS_COMPLEXITY", 10),
		OpusInBandFEC:       getEnvAsBool("OPUS_INBAND_FEC", true),
		OpusPacketLossPerc:  getEnvAsInt("OPUS_PACKET_LOSS_PERC", 15),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvAsBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return fallback
}
