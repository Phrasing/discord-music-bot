package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/zmb3/spotify"
	"golang.org/x/oauth2/clientcredentials"
)

var (
	spotifyClient spotify.Client
)

func initSpotify() {
	config := LoadConfig()

	if config.SpotifyClientID != "" && config.SpotifyClientSecret != "" {
		authConfig := &clientcredentials.Config{
			ClientID:     config.SpotifyClientID,
			ClientSecret: config.SpotifyClientSecret,
			TokenURL:     spotify.TokenURL,
		}

		accessToken, err := authConfig.Token(context.Background())
		if err != nil {
			log.Printf("error retrieving spotify access token: %v", err)
		}

		client := spotify.NewAuthenticator("").NewClient(accessToken)
		spotifyClient = client
	}
}

func getTrackName(trackID spotify.ID) (string, error) {
	track, err := spotifyClient.GetTrack(trackID)
	if err != nil {
		return "", err
	}

	artistName := ""
	if len(track.Artists) > 0 {
		artistName = track.Artists[0].Name
	}

	return fmt.Sprintf("%s - %s", track.Name, artistName), nil
}

func searchYoutube(query string) (string, error) {
	ytdlArgs := []string{
		"--get-id",
		fmt.Sprintf("ytsearch:%s", query),
	}

	ytdl := exec.Command("yt-dlp", ytdlArgs...)
	ytdlout, err := ytdl.Output()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://www.youtube.com/watch?v=%s", strings.TrimSpace(string(ytdlout))), nil
}
