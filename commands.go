package main

import "github.com/bwmarrin/discordgo"

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "play",
			Description: "Play a song from YouTube",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "url",
					Description: "The YouTube URL of the song",
					Required:    true,
				},
			},
		},
		{
			Name:        "stop",
			Description: "Stop playing music and leave the voice channel",
		},
		{
			Name:        "skip",
			Description: "Skip the current song",
		},
		{
			Name:        "pause",
			Description: "Pause or resume the current song",
		},
	}
)
