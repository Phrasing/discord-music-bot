package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/zmb3/spotify"
	"golang.org/x/oauth2/clientcredentials"
)

var (
	spotifyClient *spotify.Client
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
		spotifyClient = &client
	}
}

func getSpotifyTrack(url string) (*Song, error) {
	if spotifyClient == nil {
		return nil, fmt.Errorf("spotify client not initialized")
	}

	parts := strings.Split(url, "track/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid spotify track URL")
	}
	trackID := spotify.ID(strings.Split(parts[1], "?")[0])

	track, err := spotifyClient.GetTrack(trackID)
	if err != nil {
		return nil, fmt.Errorf("getting spotify track: %w", err)
	}

	return &Song{
		Title:    fmt.Sprintf("%s - %s", track.Artists[0].Name, track.Name),
		Duration: time.Duration(track.Duration) * time.Millisecond,
	}, nil
}

func getSpotifyPlaylist(url, channelID string) ([]*Song, error) {
	if spotifyClient == nil {
		return nil, fmt.Errorf("spotify client not initialized")
	}

	parts := strings.Split(url, "playlist/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid spotify playlist URL")
	}
	playlistID := spotify.ID(strings.Split(parts[1], "?")[0])

	playlist, err := spotifyClient.GetPlaylistTracks(playlistID)
	if err != nil {
		return nil, fmt.Errorf("getting spotify playlist: %w", err)
	}

	var songs []*Song
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, item := range playlist.Tracks {
		if item.Track.ID == "" {
			continue
		}
		wg.Add(1)
		go func(track spotify.FullTrack) {
			defer wg.Done()
			ytURL, err := searchYoutube(fmt.Sprintf("%s - %s", track.Artists[0].Name, track.Name))
			if err != nil {
				log.Printf("could not find youtube video for %s: %v", track.Name, err)
				return
			}

			mu.Lock()
			songs = append(songs, &Song{
				URL:       ytURL,
				Title:     track.Name,
				Duration:  time.Duration(track.Duration) * time.Millisecond,
				ChannelID: channelID,
			})
			mu.Unlock()
		}(item.Track)
	}

	wg.Wait()
	return songs, nil
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
