package main

import "github.com/bwmarrin/discordgo"

var (
	musicButtons = []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Pause/Resume",
					Style:    discordgo.PrimaryButton,
					CustomID: "music_pause",
				},
				discordgo.Button{
					Label:    "Skip",
					Style:    discordgo.PrimaryButton,
					CustomID: "music_skip",
				},
				discordgo.Button{
					Label:    "Stop",
					Style:    discordgo.DangerButton,
					CustomID: "music_stop",
				},
			},
		},
	}

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
