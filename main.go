package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	// A map to store the voice connections for each guild
	voiceConnections = make(map[string]*discordgo.VoiceConnection)
	// A map to store the inactivity timers for each guild
	inactivityTimers = make(map[string]*time.Timer)
	// A map to store the queues for each guild
	queues = make(map[string]*Queue)
)

func main() {
	config := LoadConfig()

	if config.BotToken == "" {
		log.Fatal("Bot token not found. Please set the BOT_TOKEN environment variable.")
	}

	dg, err := discordgo.New("Bot " + config.BotToken)
	if err != nil {
		log.Fatal("Error creating Discord session: ", err)
	}

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Bot is ready.")
	})
	dg.AddHandler(interactionCreate)

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	err = dg.Open()
	if err != nil {
		log.Fatal("Error opening connection: ", err)
	}

	log.Println("Removing old commands...")
	registeredCommands, err := dg.ApplicationCommands(dg.State.User.ID, "")
	if err != nil {
		log.Fatalf("Could not fetch registered commands: %v", err)
	}

	for _, v := range registeredCommands {
		err := dg.ApplicationCommandDelete(dg.State.User.ID, "", v.ID)
		if err != nil {
			log.Panicf("Cannot delete '%v' command: %v", v.Name, err)
		}
	}

	log.Println("Adding commands...")
	for _, v := range commands {
		_, err := dg.ApplicationCommandCreate(dg.State.User.ID, "", v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
	}
	log.Println("Commands added.")

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.Close()
}

func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type == discordgo.InteractionApplicationCommand {
		switch i.ApplicationCommandData().Name {
		case "play":
			// Respond to the interaction
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Searching for the song...",
				},
			})
			if err != nil {
				log.Println("Error responding to interaction: ", err)
			}

			videoURL := i.ApplicationCommandData().Options[0].StringValue()

			// Find the channel that the user is in
			guild, err := s.State.Guild(i.GuildID)
			if err != nil {
				log.Println("Error getting guild: ", err)
				return
			}

			var voiceChannelID string
			for _, vs := range guild.VoiceStates {
				if vs.UserID == i.Member.User.ID {
					voiceChannelID = vs.ChannelID
					break
				}
			}

			if voiceChannelID == "" {
				content := "You are not in a voice channel."
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}

			// Join the voice channel
			vc, err := s.ChannelVoiceJoin(i.GuildID, voiceChannelID, false, true)
			if err != nil {
				log.Println("Error joining voice channel: ", err)
				content := "Error joining voice channel."
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
				return
			}
			voiceConnections[i.GuildID] = vc

			// If there's an inactivity timer running, stop it
			if timer, ok := inactivityTimers[i.GuildID]; ok {
				timer.Stop()
				delete(inactivityTimers, i.GuildID)
			}

			// Get the queue for the guild
			if _, ok := queues[i.GuildID]; !ok {
				queues[i.GuildID] = NewQueue()
			}
			queue := queues[i.GuildID]

			// Add the song to the queue
			song := &Song{
				URL:       videoURL,
				ChannelID: i.ChannelID,
			}
			queue.Add(song)

			// If nothing is playing, start playing
			if len(vc.OpusSend) == 0 {
				go playNext(s, i.GuildID)
			} else {
				content := "Added to queue."
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})
			}

		case "stop":
			if vc, ok := voiceConnections[i.GuildID]; ok {
				// Clear the queue
				if queue, ok := queues[i.GuildID]; ok {
					for !queue.IsEmpty() {
						queue.Get()
					}
				}

				if timer, ok := inactivityTimers[i.GuildID]; ok {
					timer.Stop()
					delete(inactivityTimers, i.GuildID)
				}
				vc.Disconnect()
				delete(voiceConnections, i.GuildID)
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Stopped playing and left the voice channel.",
					},
				})
			} else {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Not currently in a voice channel.",
					},
				})
			}
		}
	}
}

func playNext(s *discordgo.Session, guildID string) {
	queue, ok := queues[guildID]
	if !ok || queue.IsEmpty() {
		// If the queue is empty, start the inactivity timer
		inactivityTimers[guildID] = time.AfterFunc(30*time.Second, func() {
			if vc, ok := voiceConnections[guildID]; ok {
				vc.Disconnect()
				delete(voiceConnections, guildID)
				log.Println("Disconnected due to inactivity")
			}
			delete(inactivityTimers, guildID)
		})
		return
	}

	song := queue.Get()
	playSound(s, guildID, song.ChannelID, song.URL)
}

func playSound(s *discordgo.Session, guildID, channelID, videoURL string) {
	log.Println("playSound started")
	config := LoadConfig()

	vc, ok := voiceConnections[guildID]
	if !ok {
		log.Println("Voice connection not found for guild: ", guildID)
		return
	}

	s.ChannelMessageSend(channelID, fmt.Sprintf("Now playing: %s", videoURL))

	ytdlArgs := []string{
		"--get-url",
		"-f", "bestaudio",
		"--no-playlist",
		videoURL,
	}
	if config.CookiesPath != "" {
		ytdlArgs = append(ytdlArgs, "--cookies", config.CookiesPath)
	}
	ytdl := exec.Command("yt-dlp", ytdlArgs...)
	var ytdlerr bytes.Buffer
	ytdl.Stderr = &ytdlerr
	ytdlout, err := ytdl.Output()
	if err != nil {
		log.Printf("Error getting stream URL: %v", err)
		log.Printf("yt-dlp stderr: %s", ytdlerr.String())
		s.ChannelMessageSend(channelID, "Error getting audio stream.")
		playNext(s, guildID)
		return
	}
	streamURL := strings.TrimSpace(string(ytdlout))

	ffmpeg := exec.Command("ffmpeg", "-i", streamURL, "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	ffmpegerr, err := ffmpeg.StderrPipe()
	if err != nil {
		log.Printf("Error getting ffmpeg stderr pipe: %v", err)
		playNext(s, guildID)
		return
	}

	dca := exec.Command("dca")
	dcaerr, err := dca.StderrPipe()
	if err != nil {
		log.Printf("Error getting dca stderr pipe: %v", err)
		playNext(s, guildID)
		return
	}

	ffmpegout, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Printf("Error getting ffmpeg stdout pipe: %v", err)
		playNext(s, guildID)
		return
	}
	dca.Stdin = ffmpegout

	dcaout, err := dca.StdoutPipe()
	if err != nil {
		log.Printf("Error getting dca stdout pipe: %v", err)
		playNext(s, guildID)
		return
	}

	go func() {
		scanner := bufio.NewScanner(ffmpegerr)
		for scanner.Scan() {
			log.Printf("[ffmpeg] %s", scanner.Text())
		}
	}()

	go func() {
		scanner := bufio.NewScanner(dcaerr)
		for scanner.Scan() {
			log.Printf("[dca] %s", scanner.Text())
		}
	}()

	err = ffmpeg.Start()
	if err != nil {
		log.Printf("Error starting ffmpeg: %v", err)
		playNext(s, guildID)
		return
	}
	log.Println("ffmpeg started")

	err = dca.Start()
	if err != nil {
		log.Printf("Error starting dca: %v", err)
		playNext(s, guildID)
		return
	}
	log.Println("dca started")

	vc.Speaking(true)
	defer vc.Speaking(false)

	log.Println("Reading from dca pipe")
	// Reading from the DCA stdout pipe and sending it to Discord
	var opuslen int16
	for {
		// Read opus frame length from dca file.
		err = binary.Read(dcaout, binary.LittleEndian, &opuslen)
		if err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				log.Printf("Error reading from dca stdout: %v", err)
			}
			break
		}

		// Read encoded pcm from dca file.
		InBuf := make([]byte, opuslen)
		err = binary.Read(dcaout, binary.LittleEndian, &InBuf)
		if err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				log.Printf("Error reading from dca stdout: %v", err)
			}
			break
		}

		vc.OpusSend <- InBuf
	}
	log.Println("Finished reading from dca pipe")

	ffmpeg.Wait()
	dca.Wait()

	log.Println("playSound finished")

	playNext(s, guildID)
}
