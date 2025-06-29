package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

func pow(x, y float64) float64 {
	return math.Pow(x, y)
}

type Config struct {
	// Discord Bot Configuration
	BotToken string

	// External Service Configuration
	CookiesPath         string
	SpotifyClientID     string
	SpotifyClientSecret string
	YtDlpProxy          string
	GeminiAPIKey        string
	DJPromptFilePath    string

	// Opus Encoder Settings
	OpusBitrate        int  // SetBitrate(bits int)
	OpusComplexity     int  // SetComplexity(complexity int)
	OpusInBandFEC      bool // SetInBandFEC(fec bool)
	OpusPacketLossPerc int  // SetPacketLossPerc(percentage int)
	OpusDTX            bool // SetDTX(dtx bool) - Discontinuous Transmission

	// Audio Processing Settings
	BufferSize         int     // Frame size in samples per channel
	AudioVolume        float64 // Volume multiplier (0.0-2.0)
	AudioNormalization bool    // Enable loudness normalization

	// Advanced Audio Processing
	AudioCompressor     bool    // Enable dynamic range compression
	CompressorThreshold float64 // Compressor threshold in dB (e.g., -20.0)
	CompressorRatio     float64 // Compression ratio (e.g., 4.0)
	CompressorAttack    int     // Attack time in ms (e.g., 5)
	CompressorRelease   int     // Release time in ms (e.g., 50)

	// Resampling Settings
	EnableResampling  bool // Enable high-quality resampling
	ResamplingQuality int  // SoX resampler precision (16-33)

	// FFmpeg Performance Settings
	FFmpegThreadQueueSize int    // Packet queue size
	FFmpegBufferSize      string // Output buffer size (e.g., "512k")
	FFmpegRTBufferSize    string // Real-time buffer size (e.g., "256M")
	FFmpegProbeSize       int    // Probe size in bytes
	FFmpegAnalyzeDuration int    // Analyze duration in microseconds (0 = disabled)
	FFmpegReconnectDelay  int    // Max reconnection delay in seconds

	// Quality Preset
	QualityPreset string // "performance", "balanced", "quality"
}

func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("Info: .env file not found, falling back to environment variables.")
	}

	// Load quality preset first to set defaults
	preset := getEnvAsString("QUALITY_PRESET", "balanced")

	config := &Config{
		// Discord Bot Configuration
		BotToken: os.Getenv("BOT_TOKEN"),

		// External Service Configuration
		CookiesPath:         os.Getenv("COOKIES_PATH"),
		SpotifyClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		SpotifyClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		YtDlpProxy:          os.Getenv("YT_DLP_PROXY"),
		GeminiAPIKey:        os.Getenv("GEMINI_API_KEY"),
		DJPromptFilePath:    getEnvAsString("DJ_PROMPT_FILE_PATH", "djprompt.txt"),

		// Opus Encoder Settings - Optimized for music streaming on Discord
		OpusBitrate:        getEnvAsInt("OPUS_BITRATE", 128000),     // 128kbps - Discord's max
		OpusComplexity:     getEnvAsInt("OPUS_COMPLEXITY", 10),      // 10 for best quality.
		OpusInBandFEC:      getEnvAsBool("OPUS_INBAND_FEC", true),   // Forward Error Correction
		OpusPacketLossPerc: getEnvAsInt("OPUS_PACKET_LOSS_PERC", 5), // Expected packet loss %
		OpusDTX:            getEnvAsBool("OPUS_DTX", false),         // DTX off for music

		// Audio Processing Settings
		BufferSize:         getEnvAsInt("BUFFER_SIZE", 960),           // 960 samples = 20ms @ 48kHz
		AudioVolume:        getEnvAsFloat("AUDIO_VOLUME", 1.0),        // Default: no change
		AudioNormalization: getEnvAsBool("AUDIO_NORMALIZATION", true), // EBU R128 normalization

		// Advanced Audio Processing
		AudioCompressor:     getEnvAsBool("AUDIO_COMPRESSOR", true),       // Light compression by default
		CompressorThreshold: getEnvAsFloat("COMPRESSOR_THRESHOLD", -20.0), // -20dB threshold
		CompressorRatio:     getEnvAsFloat("COMPRESSOR_RATIO", 4.0),       // 4:1 ratio
		CompressorAttack:    getEnvAsInt("COMPRESSOR_ATTACK", 5),          // 5ms attack
		CompressorRelease:   getEnvAsInt("COMPRESSOR_RELEASE", 50),        // 50ms release

		// Resampling Settings
		EnableResampling:  getEnvAsBool("ENABLE_RESAMPLING", true), // High-quality resampling
		ResamplingQuality: getEnvAsInt("RESAMPLING_QUALITY", 28),   // SoX HQ (16-33)

		// FFmpeg Performance Settings
		FFmpegThreadQueueSize: getEnvAsInt("FFMPEG_THREAD_QUEUE_SIZE", 512),
		FFmpegBufferSize:      getEnvAsString("FFMPEG_BUFFER_SIZE", "512k"),
		FFmpegRTBufferSize:    getEnvAsString("FFMPEG_RT_BUFFER_SIZE", "256M"),
		FFmpegProbeSize:       getEnvAsInt("FFMPEG_PROBE_SIZE", 32),
		FFmpegAnalyzeDuration: getEnvAsInt("FFMPEG_ANALYZE_DURATION", 0),
		FFmpegReconnectDelay:  getEnvAsInt("FFMPEG_RECONNECT_DELAY", 5),

		// Quality Preset
		QualityPreset: preset,
	}

	// Apply preset defaults
	config.applyPreset()

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

func getEnvAsFloat(key string, fallback float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
		log.Printf("Warning: Invalid float value for %s: %s, using default: %.2f", key, value, fallback)
	}
	return fallback
}

func getEnvAsString(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// applyPreset applies configuration based on quality preset
func (c *Config) applyPreset() {
	// Only apply if not overridden by environment variables
	switch c.QualityPreset {
	case "performance":
		// Optimize for low CPU usage
		if os.Getenv("OPUS_COMPLEXITY") == "" {
			c.OpusComplexity = 6
		}
		if os.Getenv("ENABLE_RESAMPLING") == "" {
			c.EnableResampling = false
		}
		if os.Getenv("AUDIO_COMPRESSOR") == "" {
			c.AudioCompressor = false
		}
		if os.Getenv("FFMPEG_BUFFER_SIZE") == "" {
			c.FFmpegBufferSize = "256k"
		}

	case "quality":
		// Maximum quality settings
		if os.Getenv("RESAMPLING_QUALITY") == "" {
			c.ResamplingQuality = 33
		}
		if os.Getenv("FFMPEG_BUFFER_SIZE") == "" {
			c.FFmpegBufferSize = "1M"
		}
		if os.Getenv("FFMPEG_RT_BUFFER_SIZE") == "" {
			c.FFmpegRTBufferSize = "512M"
		}

	case "balanced":
		// Default balanced settings (already set)
	}
}

// Validate ensures configuration values are within acceptable ranges
func (c *Config) Validate() error {
	// Validate Opus complexity (0-10)
	if c.OpusComplexity < 0 || c.OpusComplexity > 10 {
		log.Printf("Warning: OpusComplexity %d is outside valid range (0-10), using 9", c.OpusComplexity)
		c.OpusComplexity = 9
	}

	// Validate bitrate
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

	// Validate audio volume (0.0-2.0 recommended)
	if c.AudioVolume < 0.0 || c.AudioVolume > 10.0 {
		log.Printf("Warning: AudioVolume %.2f is outside safe range (0.0-10.0), using 1.0", c.AudioVolume)
		c.AudioVolume = 1.0
	}

	// Validate compressor settings
	if c.CompressorThreshold > 0 {
		log.Printf("Warning: CompressorThreshold %.2f should be negative (in dB), using -20.0", c.CompressorThreshold)
		c.CompressorThreshold = -20.0
	}

	if c.CompressorRatio < 1.0 || c.CompressorRatio > 20.0 {
		log.Printf("Warning: CompressorRatio %.2f is outside typical range (1.0-20.0), using 4.0", c.CompressorRatio)
		c.CompressorRatio = 4.0
	}

	// Validate resampling quality (16-33 for SoX)
	if c.ResamplingQuality < 16 || c.ResamplingQuality > 33 {
		log.Printf("Warning: ResamplingQuality %d is outside SoX range (16-33), using 28", c.ResamplingQuality)
		c.ResamplingQuality = 28
	}

	// Validate FFmpeg settings
	if c.FFmpegThreadQueueSize < 128 || c.FFmpegThreadQueueSize > 2048 {
		log.Printf("Warning: FFmpegThreadQueueSize %d is outside recommended range (128-2048), using 512", c.FFmpegThreadQueueSize)
		c.FFmpegThreadQueueSize = 512
	}

	if c.FFmpegReconnectDelay < 1 || c.FFmpegReconnectDelay > 60 {
		log.Printf("Warning: FFmpegReconnectDelay %d is outside reasonable range (1-60), using 5", c.FFmpegReconnectDelay)
		c.FFmpegReconnectDelay = 5
	}

	return nil
}

// BuildAudioFilter constructs the FFmpeg audio filter chain based on config
func (c *Config) BuildAudioFilter() string {
	filters := []string{}

	// Resampling (first in chain for efficiency)
	if c.EnableResampling {
		filters = append(filters, fmt.Sprintf(
			"aresample=resampler=soxr:precision=%d:dither_method=triangular",
			c.ResamplingQuality,
		))
	}

	// Dynamic range compression
	if c.AudioCompressor {
		// Convert dB to linear for threshold
		thresholdLinear := fmt.Sprintf("%.6f", 10.0/pow(20.0, -c.CompressorThreshold/20.0))
		filters = append(filters, fmt.Sprintf(
			"acompressor=threshold=%s:ratio=%.1f:attack=%d:release=%d",
			thresholdLinear,
			c.CompressorRatio,
			c.CompressorAttack,
			c.CompressorRelease,
		))
	}

	// Loudness normalization (EBU R128)
	if c.AudioNormalization {
		filters = append(filters, "loudnorm=I=-16:TP=-1.5:LRA=11")
	}

	// Volume adjustment (last in chain)
	if c.AudioVolume != 1.0 {
		filters = append(filters, fmt.Sprintf("volume=%.2f", c.AudioVolume))
	}

	if len(filters) == 0 {
		return ""
	}

	return strings.Join(filters, ",")
}
