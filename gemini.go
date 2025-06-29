package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/genai"
)

var (
	geminiClient *genai.Client
)

func initGemini() {
	config := LoadConfig()
	if config.GeminiAPIKey == "" {
		log.Println("Gemini API key not found, Gemini features will be disabled.")
		return
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		log.Printf("error creating gemini client: %v", err)
		return
	}
	geminiClient = client
}

func generateContent(prompt string) (string, error) {
	if geminiClient == nil {
		return "", fmt.Errorf("gemini client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := geminiClient.Models.GenerateContent(
		ctx,
		"gemini-2.5-pro",
		genai.Text(prompt),
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("generating content: %w", err)
	}

	return result.Text(), nil
}
