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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/zmb3/spotify"
)

var (
	voiceConnections   = make(map[string]*discordgo.VoiceConnection)
	inactivityTimers   = make(map[string]*time.Timer)
	queues             = make(map[string]*Queue)
	skipChannels       = make(map[string]chan bool)
	paused             = make(map[string]bool)
	nowPlayingMessages = make(map[string]*discordgo.Message)
	runningProcesses   = make(map[string][]*os.Process)
	queueMessages      = make(map[string]string)
)

func main() {
	initSpotify()
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
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		switch i.ApplicationCommandData().Name {
		case "play":
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Searching for the song...",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				log.Println("Error responding to interaction: ", err)
			}

			query := i.ApplicationCommandData().Options[0].StringValue()
			var videoURL string

			if strings.Contains(query, "spotify.com") {
				trackIDString := strings.Split(query, "track/")[1]
				trackID := spotify.ID(strings.Split(trackIDString, "?")[0])
				trackName, err := getTrackName(trackID)
				if err != nil {
					log.Printf("Error getting track name: %v", err)
					content := "Error getting track name from Spotify."
					s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
						Content: &content,
					})
					return
				}

				videoURL, err = searchYoutube(trackName)
				if err != nil {
					log.Printf("Error searching youtube: %v", err)
					content := "Error searching for the song on YouTube."
					s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
						Content: &content,
					})
					return
				}
			} else if strings.Contains(query, "soundcloud.com") {
				videoURL = query
			} else if !strings.HasPrefix(query, "http") {
				videoURL, err = searchYoutube(query)
				if err != nil {
					log.Printf("Error searching youtube: %v", err)
					content := "Error searching for the song on YouTube."
					s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
						Content: &content,
					})
					return
				}
			} else {
				videoURL = query
			}

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

			vc, ok := voiceConnections[i.GuildID]
			if !ok {
				vc, err = s.ChannelVoiceJoin(i.GuildID, voiceChannelID, false, true)
				if err != nil {
					log.Println("Error joining voice channel: ", err)
					content := "Error joining voice channel."
					s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
						Content: &content,
					})
					return
				}
				voiceConnections[i.GuildID] = vc
			}

			if timer, ok := inactivityTimers[i.GuildID]; ok {
				timer.Stop()
				delete(inactivityTimers, i.GuildID)
			}

			if _, ok := queues[i.GuildID]; !ok {
				queues[i.GuildID] = NewQueue()
			}
			queue := queues[i.GuildID]

			duration, err := getDuration(videoURL)
			if err != nil {
				log.Printf("Error getting duration: %v", err)
			}

			song := &Song{
				URL:       videoURL,
				ChannelID: i.ChannelID,
				Duration:  duration,
			}
			queue.Add(song)

			if len(vc.OpusSend) == 0 {
				go playNext(s, i.GuildID, nil)
			} else {
				queueList := queue.List()
				var content string
				content = "Added to queue:\n"
				for i, song := range queueList {
					content += fmt.Sprintf("%d. %s\n", i+1, song.URL)
				}
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &content,
				})

				var queueText string
				for i, song := range queueList {
					queueText += fmt.Sprintf("%d. %s\n", i+1, song.URL)
				}
				queueMessages[i.GuildID] = queueText
			}

		case "skip":
			if skip, ok := skipChannels[i.GuildID]; ok {
				skip <- true
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Skipped the current song.",
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
			} else {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Nothing to skip.",
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
			}

		case "pause":
			if vc, ok := voiceConnections[i.GuildID]; ok {
				paused[i.GuildID] = !paused[i.GuildID]
				vc.Speaking(!paused[i.GuildID])
				var status string
				if paused[i.GuildID] {
					status = "Paused"
				} else {
					status = "Resumed"
				}
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: status,
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
			} else {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Not in a voice channel.",
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
			}

		case "stop":
			if vc, ok := voiceConnections[i.GuildID]; ok {
				delete(queueMessages, i.GuildID)
				if queue, ok := queues[i.GuildID]; ok {
					for !queue.IsEmpty() {
						queue.Get()
					}
				}

				if procs, ok := runningProcesses[i.GuildID]; ok {
					for _, proc := range procs {
						if err := proc.Kill(); err != nil {
							log.Printf("Failed to kill process %d: %v", proc.Pid, err)
						}
					}
				}

				if nowPlaying, ok := nowPlayingMessages[i.GuildID]; ok {
					newContent := "Playback stopped."
					s.ChannelMessageEditComplex(&discordgo.MessageEdit{
						Content:    &newContent,
						Components: &[]discordgo.MessageComponent{},
						ID:         nowPlaying.ID,
						Channel:    nowPlaying.ChannelID,
					})
					delete(nowPlayingMessages, i.GuildID)
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
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
			} else {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Not currently in a voice channel.",
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
			}
		}
	case discordgo.InteractionMessageComponent:
		switch i.MessageComponentData().CustomID {
		case "music_pause":
			if vc, ok := voiceConnections[i.GuildID]; ok {
				paused[i.GuildID] = !paused[i.GuildID]
				vc.Speaking(!paused[i.GuildID])

				var emojiName string
				if paused[i.GuildID] {
					emojiName = "▶️"
				} else {
					emojiName = "⏸️"
				}

				components := i.Message.Components
				if len(components) > 0 {
					if row, ok := components[0].(*discordgo.ActionsRow); ok {
						for _, component := range row.Components {
							if button, ok := component.(*discordgo.Button); ok {
								if button.CustomID == "music_pause" {
									button.Emoji.Name = emojiName
								}
							}
						}
					}
				}

				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Content:    i.Message.Content,
						Components: components,
					},
				})
			}
		case "music_skip":
			if skip, ok := skipChannels[i.GuildID]; ok {
				skip <- true
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Content:    "Skipped the current song.",
						Components: musicButtons,
					},
				})
			}
		case "music_stop":
			if vc, ok := voiceConnections[i.GuildID]; ok {
				delete(queueMessages, i.GuildID)
				if queue, ok := queues[i.GuildID]; ok {
					for !queue.IsEmpty() {
						queue.Get()
					}
				}

				if procs, ok := runningProcesses[i.GuildID]; ok {
					for _, proc := range procs {
						if err := proc.Kill(); err != nil {
							log.Printf("Failed to kill process %d: %v", proc.Pid, err)
						}
					}
				}

				if nowPlaying, ok := nowPlayingMessages[i.GuildID]; ok {
					newContent := "Playback stopped."
					s.ChannelMessageEditComplex(&discordgo.MessageEdit{
						Content:    &newContent,
						Components: &[]discordgo.MessageComponent{},
						ID:         nowPlaying.ID,
						Channel:    nowPlaying.ChannelID,
					})
					delete(nowPlayingMessages, i.GuildID)
				}

				if timer, ok := inactivityTimers[i.GuildID]; ok {
					timer.Stop()
					delete(inactivityTimers, i.GuildID)
				}
				vc.Disconnect()
				delete(voiceConnections, i.GuildID)
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Content: "Stopped playing and left the voice channel.",
					},
				})
			}
		}
	}
}

func playNext(s *discordgo.Session, guildID string, lastSong *Song) {
	queue, ok := queues[guildID]
	if !ok || queue.IsEmpty() {
		delete(queueMessages, guildID)
		if lastSong != nil {
			if nowPlaying, ok := nowPlayingMessages[guildID]; ok {
				newContent := fmt.Sprintf("Playback Finished: %s", lastSong.URL)
				s.ChannelMessageEditComplex(&discordgo.MessageEdit{
					Content:    &newContent,
					Components: &[]discordgo.MessageComponent{},
					ID:         nowPlaying.ID,
					Channel:    nowPlaying.ChannelID,
				})
				delete(nowPlayingMessages, guildID)
			}
		}

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
	playSound(s, guildID, song)
}

func playSound(s *discordgo.Session, guildID string, song *Song) {
	log.Println("playSound started")
	config := LoadConfig()

	videoURL := song.URL
	channelID := song.ChannelID

	vc, ok := voiceConnections[guildID]
	if !ok {
		log.Println("Voice connection not found for guild: ", guildID)
		return
	}

	components := musicButtons
	if queues[guildID].IsEmpty() {
		components = musicButtonsNoSkip
	}

	content := fmt.Sprintf("Now playing: %s", videoURL)
	if song.Duration > 0 {
		content += fmt.Sprintf("\n`%s / %s`", formatDuration(0), formatDuration(song.Duration))
	}
	nowPlaying, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:    content,
		Components: components,
	})
	nowPlayingURL := videoURL

	if err == nil {
		nowPlayingMessages[guildID] = nowPlaying
	}

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
		playNext(s, guildID, song)
		return
	}
	streamURL := strings.TrimSpace(string(ytdlout))

	ffmpeg := exec.Command("ffmpeg", "-nostdin", "-i", streamURL, "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	ffmpegerr, err := ffmpeg.StderrPipe()
	if err != nil {
		log.Printf("Error getting ffmpeg stderr pipe: %v", err)
		playNext(s, guildID, song)
		return
	}

	dca := exec.Command("dca")
	dcaerr, err := dca.StderrPipe()
	if err != nil {
		log.Printf("Error getting dca stderr pipe: %v", err)
		playNext(s, guildID, song)
		return
	}

	ffmpegout, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Printf("Error getting ffmpeg stdout pipe: %v", err)
		playNext(s, guildID, song)
		return
	}
	dca.Stdin = ffmpegout

	dcaout, err := dca.StdoutPipe()
	if err != nil {
		log.Printf("Error getting dca stdout pipe: %v", err)
		playNext(s, guildID, song)
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
		playNext(s, guildID, song)
		return
	}
	log.Println("ffmpeg started")

	err = dca.Start()
	if err != nil {
		log.Printf("Error starting dca: %v", err)
		playNext(s, guildID, song)
		return
	}
	log.Println("dca started")

	runningProcesses[guildID] = []*os.Process{ffmpeg.Process, dca.Process}
	defer delete(runningProcesses, guildID)

	vc.Speaking(true)
	defer vc.Speaking(false)

	skip := make(chan bool)
	skipChannels[guildID] = skip
	done := make(chan bool)

	log.Println("Reading from dca pipe")

	ticker := time.NewTicker(1 * time.Second)
	startTime := time.Now()
	var pausedTime time.Time
	var totalPausedDuration time.Duration

	go func() {
		for {
			select {
			case <-ticker.C:
				if paused[guildID] {
					if pausedTime.IsZero() {
						pausedTime = time.Now()
					}
					continue
				} else {
					if !pausedTime.IsZero() {
						totalPausedDuration += time.Since(pausedTime)
						pausedTime = time.Time{}
					}
				}

				elapsed := time.Since(startTime) - totalPausedDuration
				if nowPlaying, ok := nowPlayingMessages[guildID]; ok {
					newContent := fmt.Sprintf("Now playing: %s\n`%s / %s`",
						nowPlayingURL,
						formatDuration(elapsed),
						formatDuration(song.Duration),
					)
					if queueText, ok := queueMessages[guildID]; ok {
						newContent += "\n\n**Queue:**\n" + queueText
					}
					s.ChannelMessageEdit(nowPlaying.ChannelID, nowPlaying.ID, newContent)
				}
			case <-done:
				return
			}
		}
	}()

	var opuslen int16
readLoop:
	for {
		select {
		case <-skip:
			log.Println("Song skipped")
			close(done)
			ticker.Stop()
			ffmpeg.Process.Kill()
			dca.Process.Kill()
			playNext(s, guildID, song)
			return
		default:
			if paused[guildID] {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			err = binary.Read(dcaout, binary.LittleEndian, &opuslen)
			if err != nil {
				if err != io.EOF && err != io.ErrUnexpectedEOF {
					log.Printf("Error reading from dca stdout: %v", err)
				}
				break readLoop
			}

			InBuf := make([]byte, opuslen)
			err = binary.Read(dcaout, binary.LittleEndian, &InBuf)
			if err != nil {
				if err != io.EOF && err != io.ErrUnexpectedEOF {
					log.Printf("Error reading from dca stdout: %v", err)
				}
				break readLoop
			}

			vc.OpusSend <- InBuf
		}
	}

	close(done)
	ticker.Stop()
	ffmpeg.Wait()
	dca.Wait()

	log.Println("playSound finished")

	playNext(s, guildID, song)
}

func getDuration(videoURL string) (time.Duration, error) {
	ytdl := exec.Command("yt-dlp", "--get-duration", videoURL)
	durationBytes, err := ytdl.Output()
	if err != nil {
		return 0, err
	}

	durationStr := strings.TrimSpace(string(durationBytes))
	parts := strings.Split(durationStr, ":")
	var duration time.Duration
	if len(parts) == 3 { // HH:MM:SS
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		s, _ := strconv.Atoi(parts[2])
		duration = time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
	} else if len(parts) == 2 { // MM:SS
		m, _ := strconv.Atoi(parts[0])
		s, _ := strconv.Atoi(parts[1])
		duration = time.Duration(m)*time.Minute + time.Duration(s)*time.Second
	} else if len(parts) == 1 { // SS
		s, _ := strconv.Atoi(parts[0])
		duration = time.Duration(s) * time.Second
	} else {
		return 0, fmt.Errorf("invalid duration format: %s", durationStr)
	}

	return duration, nil
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%02d:%02d", m, s)
}
