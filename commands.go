package main

import "github.com/bwmarrin/discordgo"

var (
	musicButtons = []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Emoji: &discordgo.ComponentEmoji{
						Name: "⏸️",
					},
					Style:    discordgo.SecondaryButton,
					CustomID: "music_pause",
				},
				discordgo.Button{
					Emoji: &discordgo.ComponentEmoji{
						Name: "⏭️",
					},
					Style:    discordgo.SecondaryButton,
					CustomID: "music_skip",
				},
				discordgo.Button{
					Emoji: &discordgo.ComponentEmoji{
						Name: "⏹️",
					},
					Style:    discordgo.SecondaryButton,
					CustomID: "music_stop",
				},
			},
		},
	}

	musicButtonsNoSkip = []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Emoji: &discordgo.ComponentEmoji{
						Name: "⏸️",
					},
					Style:    discordgo.SecondaryButton,
					CustomID: "music_pause",
				},
				discordgo.Button{
					Emoji: &discordgo.ComponentEmoji{
						Name: "⏹️",
					},
					Style:    discordgo.SecondaryButton,
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
		{
			Name:        "ask",
			Description: "Ask a question to the Gemini AI",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "prompt",
					Description: "The question you want to ask",
					Required:    true,
				},
			},
		},
		{
			Name:        "dj",
			Description: "Let the AI DJ play a set for you",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "genre",
					Description: "The genre you want to listen to",
					Required:    true,
				},
			},
		},
	}
)
