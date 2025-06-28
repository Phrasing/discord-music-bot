package main

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	// Discord Bot Configuration
	BotToken string

	// External Service Configuration
	CookiesPath         string
	SpotifyClientID     string
	SpotifyClientSecret string
	YtDlpProxy          string

	// Opus Encoder Settings (all available methods)
	OpusBitrate        int  // SetBitrate(bits int)
	OpusComplexity     int  // SetComplexity(complexity int)
	OpusInBandFEC      bool // SetInBandFEC(fec bool)
	OpusPacketLossPerc int  // SetPacketLossPerc(percentage int)
	OpusDTX            bool // SetDTX(dtx bool) - Discontinuous Transmission

	// Audio Processing Settings
	BufferSize int // Frame size in samples per channel
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables")
	}

	config := &Config{
		// Discord Bot Configuration
		BotToken: os.Getenv("BOT_TOKEN"),

		// External Service Configuration
		CookiesPath:         os.Getenv("COOKIES_PATH"),
		SpotifyClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		SpotifyClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		YtDlpProxy:          os.Getenv("YT_DLP_PROXY"),

		// Opus Encoder Settings - Optimized for music streaming on Discord
		OpusBitrate:        getEnvAsInt("OPUS_BITRATE", 128000),     // 128kbps - Discord's max for boosted servers
		OpusComplexity:     getEnvAsInt("OPUS_COMPLEXITY", 9),       // 0-10: 9 is nearly as good as 10 with less CPU
		OpusInBandFEC:      getEnvAsBool("OPUS_INBAND_FEC", true),   // Forward Error Correction - important for streaming
		OpusPacketLossPerc: getEnvAsInt("OPUS_PACKET_LOSS_PERC", 5), // Expected packet loss % - 5% is good balance
		OpusDTX:            getEnvAsBool("OPUS_DTX", false),         // DTX off for music (only useful for speech)

		// Audio Processing Settings
		BufferSize: getEnvAsInt("BUFFER_SIZE", 960), // 960 samples = 20ms @ 48kHz
	}

	// Validate configuration
	config.Validate()

	return config
}

func getEnvAsInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
		log.Printf("Warning: Invalid integer value for %s: %s, using default: %d", key, value, fallback)
	}
	return fallback
}

func getEnvAsBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
		log.Printf("Warning: Invalid boolean value for %s: %s, using default: %v", key, value, fallback)
	}
	return fallback
}

// Validate ensures configuration values are within acceptable ranges
func (c *Config) Validate() error {
	// Validate Opus complexity (0-10)
	if c.OpusComplexity < 0 || c.OpusComplexity > 10 {
		log.Printf("Warning: OpusComplexity %d is outside valid range (0-10), using 9", c.OpusComplexity)
		c.OpusComplexity = 9
	}

	// Validate bitrate (6-510 kbps per channel, so 12-1020 kbps for stereo)
	// Discord limits: 96kbps (normal) or 128kbps (boosted)
	if c.OpusBitrate < 12000 || c.OpusBitrate > 128000 {
		log.Printf("Warning: OpusBitrate %d is outside Discord range (12000-128000), using 128000", c.OpusBitrate)
		c.OpusBitrate = 128000
	}

	// Validate packet loss percentage (0-100)
	if c.OpusPacketLossPerc < 0 || c.OpusPacketLossPerc > 100 {
		log.Printf("Warning: OpusPacketLossPerc %d is outside valid range (0-100), using 5", c.OpusPacketLossPerc)
		c.OpusPacketLossPerc = 5
	}

	// Validate buffer size for 48kHz
	// Common frame sizes at 48kHz: 120 (2.5ms), 240 (5ms), 480 (10ms), 960 (20ms), 1920 (40ms), 2880 (60ms)
	validBufferSizes := map[int]string{
		120:  "2.5ms",
		240:  "5ms",
		480:  "10ms",
		960:  "20ms",
		1920: "40ms",
		2880: "60ms",
	}

	if _, ok := validBufferSizes[c.BufferSize]; !ok {
		log.Printf("Warning: BufferSize %d is not a standard Opus frame size, using 960 (20ms)", c.BufferSize)
		c.BufferSize = 960
	}

	return nil
}
