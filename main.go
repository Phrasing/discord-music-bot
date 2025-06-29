package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"gopkg.in/hraban/opus.v2"
)

type Bot struct {
	session *discordgo.Session
	guilds  map[string]*GuildState
	mu      sync.RWMutex
}

type GuildState struct {
	voice         *discordgo.VoiceConnection
	queue         *Queue
	skipChan      chan bool
	done          chan bool
	paused        bool
	nowPlaying    *discordgo.Message
	process       *os.Process
	inactiveTimer *time.Timer
	mu            sync.Mutex
}

type VideoInfo struct {
	URL      string        `json:"url"`
	Title    string        `json:"title"`
	Duration time.Duration `json:"duration"`
}

func main() {
	if err := setupLogging(); err != nil {
		log.Fatal(err)
	}

	initSpotify()
	initGemini()

	config := LoadConfig()

	bot, err := NewBot(config.BotToken)
	if err != nil {
		log.Fatal(err)
	}

	if err := bot.Start(); err != nil {
		log.Fatal(err)
	}

	go checkYtDlpUpdates()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	bot.Stop()
}

func NewBot(token string) (*Bot, error) {
	if token == "" {
		return nil, fmt.Errorf("bot token not found")
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("creating discord session: %w", err)
	}

	bot := &Bot{
		session: dg,
		guilds:  make(map[string]*GuildState),
	}

	dg.AddHandler(bot.ready)
	dg.AddHandler(bot.interactionCreate)
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	return bot, nil
}

func (b *Bot) Start() error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("opening connection: %w", err)
	}

	log.Println("Registering commands...")
	if _, err := b.session.ApplicationCommandBulkOverwrite(b.session.State.User.ID, "", commands); err != nil {
		return fmt.Errorf("registering commands: %w", err)
	}

	return nil
}

func (b *Bot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for guildID, state := range b.guilds {
		state.cleanup()
		delete(b.guilds, guildID)
	}

	b.session.Close()
}

func (b *Bot) getOrCreateGuildState(guildID string) *GuildState {
	b.mu.Lock()
	defer b.mu.Unlock()

	if state, ok := b.guilds[guildID]; ok {
		return state
	}

	state := &GuildState{
		queue: NewQueue(),
	}
	b.guilds[guildID] = state
	return state
}

func (b *Bot) ready(s *discordgo.Session, r *discordgo.Ready) {
	log.Println("Bot is ready")
}

func (b *Bot) handleComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	state := b.getOrCreateGuildState(i.GuildID)

	switch i.MessageComponentData().CustomID {
	case "music_pause":
		b.handlePauseButton(s, i, state)
	case "music_skip":
		b.handleSkipButton(s, i, state)
	case "music_stop":
		b.handleStopButton(s, i, state)
	}
}

func (b *Bot) handleSkip(s *discordgo.Session, i *discordgo.InteractionCreate) {
	state := b.getOrCreateGuildState(i.GuildID)

	if state.skipChan == nil {
		respondEphemeral(s, i, "Nothing to skip")
		return
	}

	// Acknowledge the interaction immediately.
	respondEphemeral(s, i, "Skipped the current song")

	state.mu.Lock()
	if state.paused {
		state.paused = false
	}
	state.mu.Unlock()
	state.voice.Speaking(true)

	// Non-blocking send to the skip channel.
	select {
	case state.skipChan <- true:
	default:
		// If the channel is full, a skip is already pending.
	}
}

func (b *Bot) handlePause(s *discordgo.Session, i *discordgo.InteractionCreate) {
	state := b.getOrCreateGuildState(i.GuildID)
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.voice == nil {
		respondEphemeral(s, i, "Not in a voice channel")
		return
	}

	state.paused = !state.paused
	state.voice.Speaking(!state.paused)

	status := "Resumed"
	if state.paused {
		status = "Paused"
	}
	respondEphemeral(s, i, status)
}

func (b *Bot) handleStop(s *discordgo.Session, i *discordgo.InteractionCreate) {
	state := b.getOrCreateGuildState(i.GuildID)
	respondEphemeral(s, i, "Stopped playing and left the voice channel")

	state.stopPlayback(s)
	b.disconnectFromGuild(i.GuildID)
}

func (b *Bot) handlePauseButton(s *discordgo.Session, i *discordgo.InteractionCreate, state *GuildState) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.voice == nil {
		return
	}

	state.paused = !state.paused
	state.voice.Speaking(!state.paused)

	// Update button emoji
	emojiName := "⏸️"
	if state.paused {
		emojiName = "▶️"
	}

	components := i.Message.Components
	if len(components) > 0 {
		if row, ok := components[0].(*discordgo.ActionsRow); ok {
			for _, component := range row.Components {
				if button, ok := component.(*discordgo.Button); ok && button.CustomID == "music_pause" {
					button.Emoji.Name = emojiName
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

func (b *Bot) handleSkipButton(s *discordgo.Session, i *discordgo.InteractionCreate, state *GuildState) {
	if state.skipChan == nil {
		return // Or respond with an error
	}

	// Acknowledge the interaction immediately.
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})

	state.mu.Lock()
	if state.paused {
		state.paused = false
		state.voice.Speaking(true)
	}
	state.mu.Unlock()

	// Non-blocking send to the skip channel.
	select {
	case state.skipChan <- true:
	default:
		// If the channel is full, a skip is already pending.
	}
}

func (b *Bot) handleStopButton(s *discordgo.Session, i *discordgo.InteractionCreate, state *GuildState) {
	// Clear queue
	for !state.queue.IsEmpty() {
		state.queue.Get()
	}

	// Kill process
	if state.process != nil {
		state.process.Kill()
	}

	// Update message
	newContent := "Playback stopped."
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: newContent,
		},
	})

	// Disconnect
	b.disconnectFromGuild(i.GuildID)
}

func (b *Bot) disconnectFromGuild(guildID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if state, ok := b.guilds[guildID]; ok {
		state.cleanup()
		delete(b.guilds, guildID)
	}
}

func (b *Bot) updateNowPlaying(s *discordgo.Session, state *GuildState, song *Song, done <-chan bool) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	var pausedTime time.Time
	var totalPausedDuration time.Duration

	for {
		select {
		case <-ticker.C:
			state.mu.Lock()
			if state.nowPlaying == nil {
				state.mu.Unlock()
				return
			}
			paused := state.paused
			state.mu.Unlock()

			// Handle pause timing
			if paused {
				if pausedTime.IsZero() {
					pausedTime = time.Now()
				}
				continue
			} else if !pausedTime.IsZero() {
				totalPausedDuration += time.Since(pausedTime)
				pausedTime = time.Time{}
			}

			content := formatNowPlaying(song, time.Since(startTime)-totalPausedDuration)

			// Add queue info
			queueList := state.queue.List()
			if len(queueList) > 0 {
				content += "\n\n**Queue:**\n"
				for i, qSong := range queueList {
					if i >= 5 {
						content += fmt.Sprintf("... and %d more", len(queueList)-5)
						break
					}
					content += fmt.Sprintf("%d. %s\n", i+1, qSong.Title)
				}
			}

			// Determine components
			components := musicButtonsNoSkip
			if !state.queue.IsEmpty() {
				components = musicButtons
			}

			s.ChannelMessageEditComplex(&discordgo.MessageEdit{
				Content:    &content,
				Components: &components,
				ID:         state.nowPlaying.ID,
				Channel:    state.nowPlaying.ChannelID,
			})

		case <-done:
			return
		}
	}
}

func resolveSpotifyURL(url, channelID string) ([]*Song, error) {
	if strings.Contains(url, "track") {
		// It's a single track
		track, err := getSpotifyTrack(url)
		if err != nil {
			return nil, err
		}
		return []*Song{track}, nil
	} else if strings.Contains(url, "playlist") {
		// It's a playlist
		return getSpotifyPlaylist(url, channelID)
	}
	return nil, fmt.Errorf("unsupported Spotify URL")
}

func (b *Bot) interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		b.handleCommand(s, i)
	case discordgo.InteractionMessageComponent:
		b.handleComponent(s, i)
	}
}

func (b *Bot) handleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case "play":
		b.handlePlay(s, i)
	case "skip":
		b.handleSkip(s, i)
	case "pause":
		b.handlePause(s, i)
	case "stop":
		b.handleStop(s, i)
	case "ask":
		b.handleAsk(s, i)
	case "dj":
		b.handleDJ(s, i)
	}
}

func (b *Bot) handleAsk(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Defer the response immediately to avoid interaction timeout.
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Thinking...",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("could not defer response: %v", err)
		return
	}

	// Run the long-running Gemini API call in a goroutine.
	go func() {
		prompt := i.ApplicationCommandData().Options[0].StringValue()

		response, err := generateContent(prompt)
		if err != nil {
			editResponse(s, i, fmt.Sprintf("Error: %v", err))
			return
		}

		// Edit the original deferred response with the result.
		editResponse(s, i, response)
	}()
}

func (b *Bot) handlePlay(s *discordgo.Session, i *discordgo.InteractionCreate) {
	respondEphemeral(s, i, "Processing...")
	query := i.ApplicationCommandData().Options[0].StringValue()

	voiceChannelID := getUserVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if voiceChannelID == "" {
		editResponse(s, i, "You must be in a voice channel")
		return
	}

	songs, err := b.resolveQuery(query, i.ChannelID)
	if err != nil {
		editResponse(s, i, fmt.Sprintf("Error: %v", err))
		return
	}

	b.enqueueAndPlay(s, i, songs)
}

func (b *Bot) handleDJ(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "The AI DJ is crafting a set for you...",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("could not defer response: %v", err)
		return
	}

	go func() {
		config := LoadConfig()
		promptTemplate, err := os.ReadFile(config.DJPromptFilePath)
		if err != nil {
			editResponse(s, i, "Error: could not load DJ prompt file.")
			log.Printf("Error reading DJ prompt file: %v", err)
			return
		}

		userInput := i.ApplicationCommandData().Options[0].StringValue()
		prompt := fmt.Sprintf(string(promptTemplate), userInput)

		response, err := generateContent(prompt)
		if err != nil {
			editResponse(s, i, fmt.Sprintf("Error generating playlist: %v", err))
			return
		}

		songQueries := strings.Split(strings.TrimSpace(response), "\n")
		var songs []*Song
		var wg sync.WaitGroup
		var mu sync.Mutex

		for _, query := range songQueries {
			wg.Add(1)
			go func(q string) {
				defer wg.Done()
				resolvedSongs, err := b.resolveQuery(q, i.ChannelID)
				if err != nil {
					log.Printf("could not resolve song query '%s': %v", q, err)
					return
				}
				mu.Lock()
				songs = append(songs, resolvedSongs...)
				mu.Unlock()
			}(query)
		}
		wg.Wait()

		if len(songs) == 0 {
			editResponse(s, i, "Could not find any songs for that prompt.")
			return
		}

		// Shuffle the playlist
		rand.Shuffle(len(songs), func(i, j int) {
			songs[i], songs[j] = songs[j], songs[i]
		})

		b.enqueueAndPlay(s, i, songs)
	}()
}

func (b *Bot) enqueueAndPlay(s *discordgo.Session, i *discordgo.InteractionCreate, songs []*Song) {
	state := b.getOrCreateGuildState(i.GuildID)

	voiceChannelID := getUserVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if voiceChannelID == "" {
		editResponse(s, i, "You must be in a voice channel")
		return
	}

	if err := b.ensureVoiceConnection(s, i.GuildID, voiceChannelID, state); err != nil {
		editResponse(s, i, "Error joining voice channel")
		return
	}

	for _, song := range songs {
		state.queue.Add(song)
	}

	if state.process == nil {
		if len(songs) > 1 {
			editResponse(s, i, fmt.Sprintf("Added %d songs to the queue.", len(songs)))
		} else {
			editResponse(s, i, fmt.Sprintf("Playing: %s", songs[0].Title))
		}
		go b.playNext(s, i.GuildID, nil)
	} else {
		if len(songs) > 1 {
			editResponse(s, i, fmt.Sprintf("Added %d songs to the queue.", len(songs)))
		} else {
			editResponse(s, i, fmt.Sprintf("Added to queue: %s", songs[0].Title))
		}
	}
}

func (b *Bot) resolveQuery(query, channelID string) ([]*Song, error) {
	var songs []*Song

	// Handle Spotify URLs first
	if strings.Contains(query, "spotify.com") {
		return resolveSpotifyURL(query, channelID)
	}

	// Check if it's a playlist or a single video
	isPlaylist := strings.Contains(query, "list=") || strings.Contains(query, "/playlist/")

	videoInfos, err := getVideoInfos(query, isPlaylist)
	if err != nil {
		// If fetching as a playlist fails, try as a single video/search
		if isPlaylist {
			videoInfos, err = getVideoInfos(query, false)
		}
		if err != nil {
			return nil, fmt.Errorf("getting video info: %w", err)
		}
	}

	for _, info := range videoInfos {
		songs = append(songs, &Song{
			URL:       info.URL,
			Title:     info.Title,
			Duration:  info.Duration,
			ChannelID: channelID,
		})
	}

	return songs, nil
}

func (b *Bot) ensureVoiceConnection(s *discordgo.Session, guildID, channelID string, state *GuildState) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.voice != nil {
		return nil
	}

	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return err
	}

	state.voice = vc
	state.cancelInactivityTimer()
	return nil
}

func (b *Bot) playNext(s *discordgo.Session, guildID string, lastSong *Song) {
	state := b.getOrCreateGuildState(guildID)

	song := state.queue.Get()
	if song == nil {
		state.stopPlayback(s)
		state.startInactivityTimer(func() {
			b.disconnectFromGuild(guildID)
		})
		return
	}

	b.playSound(s, guildID, song)
}

func (b *Bot) playSound(s *discordgo.Session, guildID string, song *Song) {
	state := b.getOrCreateGuildState(guildID)
	config := LoadConfig()

	components := musicButtons
	if state.queue.IsEmpty() {
		components = musicButtonsNoSkip
	}

	content := formatNowPlaying(song, 0)

	var msg *discordgo.Message
	var err error

	if state.nowPlaying != nil {
		msg, err = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Content:    &content,
			Components: &components,
			ID:         state.nowPlaying.ID,
			Channel:    state.nowPlaying.ChannelID,
		})
	} else {
		msg, err = s.ChannelMessageSendComplex(song.ChannelID, &discordgo.MessageSend{
			Content:    content,
			Components: components,
		})
	}

	if err != nil {
		log.Printf("Error sending/editing now playing message: %v", err)
	} else {
		state.nowPlaying = msg
	}

	streamURL, err := getStreamURL(song.URL, config)
	if err != nil {
		log.Printf("Error getting stream URL: %v", err)
		s.ChannelMessageSend(song.ChannelID, "Error getting audio stream.")
		b.playNext(s, guildID, song)
		return
	}

	ffmpegArgs := []string{
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", fmt.Sprintf("%d", config.FFmpegReconnectDelay),
		"-nostdin",
		"-i", streamURL,
		"-f", "s16le",
		"-ar", "48000",
		"-ac", "2",
	}

	audioFilter := config.BuildAudioFilter()
	if audioFilter != "" {
		ffmpegArgs = append(ffmpegArgs, "-af", audioFilter)
	}

	ffmpegArgs = append(ffmpegArgs, "pipe:1")

	ffmpeg := exec.Command("ffmpeg", ffmpegArgs...)

	// Set process priority for real-time audio
	ffmpeg.SysProcAttr = &syscall.SysProcAttr{
		// On Linux, set nice value for higher priority
		// Nice: -10, // Requires root
	}

	ffmpegErr, err := ffmpeg.StderrPipe()
	if err != nil {
		log.Printf("Error getting ffmpeg stderr pipe: %v", err)
		b.playNext(s, guildID, song)
		return
	}

	ffmpegOut, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Printf("Error getting ffmpeg stdout pipe: %v", err)
		b.playNext(s, guildID, song)
		return
	}

	// Enhanced error logging with filter
	go func() {
		scanner := bufio.NewScanner(ffmpegErr)
		for scanner.Scan() {
			line := scanner.Text()
			// Filter out common non-error messages
			if !strings.Contains(line, "Press [q] to stop") &&
				!strings.Contains(line, "size=") &&
				!strings.Contains(line, "time=") {
				log.Printf("[ffmpeg] %s", line)
			}
		}
	}()

	if err := ffmpeg.Start(); err != nil {
		log.Printf("Error starting ffmpeg: %v", err)
		b.playNext(s, guildID, song)
		return
	}
	log.Println("ffmpeg started with optimized settings")

	state.mu.Lock()
	state.process = ffmpeg.Process
	state.skipChan = make(chan bool, 1)
	if state.done != nil {
		close(state.done)
	}
	state.done = make(chan bool)
	state.mu.Unlock()

	go b.updateNowPlaying(s, state, song, state.done)

	b.streamAudio(state.voice, ffmpegOut, state, config)

	ffmpeg.Wait()

	state.mu.Lock()
	state.process = nil
	if state.skipChan != nil {
		close(state.skipChan)
		state.skipChan = nil
	}
	state.mu.Unlock()

	log.Println("playSound finished")

	b.playNext(s, guildID, song)
}

func createOpusEncoder(config *Config) (*opus.Encoder, error) {
	// 2049 = OPUS_APPLICATION_AUDIO (best for music)
	encoder, err := opus.NewEncoder(48000, 2, opus.Application(2049))
	if err != nil {
		return nil, err
	}

	encoder.SetBitrate(config.OpusBitrate)
	encoder.SetComplexity(config.OpusComplexity)
	encoder.SetInBandFEC(config.OpusInBandFEC)
	encoder.SetPacketLossPerc(config.OpusPacketLossPerc)
	encoder.SetDTX(config.OpusDTX)

	return encoder, nil
}

func (b *Bot) streamAudio(vc *discordgo.VoiceConnection, audio io.ReadCloser, state *GuildState, config *Config) {
	const (
		channels  = 2
		frameRate = 48000
		frameSize = 960
		maxBytes  = (frameSize * channels * 2)
	)
	defer audio.Close()

	encoder, err := createOpusEncoder(config)
	if err != nil {
		log.Printf("Error creating opus encoder: %v", err)
		return
	}

	vc.Speaking(true)
	defer vc.Speaking(false)

readLoop:
	for {
		select {
		case <-state.skipChan:
			log.Println("Song skipped")
			return
		default:
			state.mu.Lock()
			paused := state.paused
			state.mu.Unlock()

			if paused {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Read PCM data exactly as original
			pcm := make([]int16, frameSize*channels)
			err = binary.Read(audio, binary.LittleEndian, &pcm)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break readLoop
			}
			if err != nil {
				log.Printf("Error reading from ffmpeg stdout: %v", err)
				break readLoop
			}

			// Encode to opus exactly as original
			opusData := make([]byte, maxBytes)
			n, err := encoder.Encode(pcm, opusData)
			if err != nil {
				log.Printf("Error encoding pcm to opus: %v", err)
				break readLoop
			}

			// Send opus data
			vc.OpusSend <- opusData[:n]
		}
	}
}

// Helper functions
func setupLogging() error {
	f, err := os.OpenFile("bot.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	return nil
}

func checkYtDlpUpdates() {
	update := func() {
		log.Println("Checking for yt-dlp updates...")
		cmd := exec.Command("pipx", "upgrade", "--pip-args=--pre", "yt-dlp")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Error updating yt-dlp: %v\n%s", err, output)
		} else {
			log.Println("Successfully checked for yt-dlp updates.")
		}
	}

	update()
	ticker := time.NewTicker(24 * time.Hour)
	for range ticker.C {
		update()
	}
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func editResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
}

func getVideoInfos(query string, isPlaylist bool) ([]*VideoInfo, error) {
	args := []string{"--dump-json"}
	if isPlaylist {
		args = append(args, "--flat-playlist")
	} else {
		args = append(args, "--no-playlist")
	}

	if !strings.HasPrefix(query, "http") {
		args = append(args, fmt.Sprintf("ytsearch:%s", query))
	} else {
		args = append(args, query)
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var infos []*VideoInfo
	if isPlaylist {
		scanner := bufio.NewScanner(strings.NewReader(string(output)))
		for scanner.Scan() {
			var data struct {
				ID       string  `json:"id"`
				Title    string  `json:"title"`
				Duration float64 `json:"duration"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &data); err != nil {
				log.Printf("Skipping unparsable playlist item: %v", err)
				continue
			}
			infos = append(infos, &VideoInfo{
				URL:      "https://www.youtube.com/watch?v=" + data.ID,
				Title:    data.Title,
				Duration: time.Duration(data.Duration * float64(time.Second)),
			})
		}
	} else {
		var data struct {
			URL      string  `json:"webpage_url"`
			Title    string  `json:"title"`
			Duration float64 `json:"duration"`
		}
		if err := json.Unmarshal(output, &data); err != nil {
			return nil, fmt.Errorf("failed to parse video info: %w", err)
		}
		infos = append(infos, &VideoInfo{
			URL:      data.URL,
			Title:    data.Title,
			Duration: time.Duration(data.Duration * float64(time.Second)),
		})
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("no video information found")
	}

	return infos, nil
}

func getStreamURL(videoURL string, config *Config) (string, error) {
	args := []string{
		"--get-url",
		"-f", "bestaudio",
		"--no-playlist",
		videoURL,
	}

	if config.CookiesPath != "" {
		args = append(args, "--cookies", config.CookiesPath)
	}
	if config.YtDlpProxy != "" {
		args = append(args, "--proxy", config.YtDlpProxy)
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func getUserVoiceChannel(s *discordgo.Session, guildID, userID string) string {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		return ""
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			return vs.ChannelID
		}
	}
	return ""
}

func formatNowPlaying(song *Song, elapsed time.Duration) string {
	return fmt.Sprintf("Now playing: [%s](%s)\n`%s / %s`",
		song.Title,
		song.URL,
		formatDuration(elapsed),
		formatDuration(song.Duration),
	)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	s := d % time.Minute / time.Second
	return fmt.Sprintf("%02d:%02d", m, s)
}

// GuildState methods

func (gs *GuildState) stopPlayback(s *discordgo.Session) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	if gs.done != nil {
		close(gs.done)
		gs.done = nil
	}

	for !gs.queue.IsEmpty() {
		gs.queue.Get()
	}

	if gs.process != nil {
		gs.process.Kill()
		gs.process = nil
	}

	if gs.nowPlaying != nil {
		newContent := "Playback stopped."
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Content:    &newContent,
			Components: &[]discordgo.MessageComponent{},
			ID:         gs.nowPlaying.ID,
			Channel:    gs.nowPlaying.ChannelID,
		})
		gs.nowPlaying = nil
	}
}

func (gs *GuildState) cleanup() {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	if gs.process != nil {
		gs.process.Kill()
	}
	if gs.voice != nil {
		gs.voice.Disconnect()
	}
	if gs.inactiveTimer != nil {
		gs.inactiveTimer.Stop()
	}
}

func (gs *GuildState) startInactivityTimer(callback func()) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	gs.cancelInactivityTimer()
	gs.inactiveTimer = time.AfterFunc(30*time.Second, callback)
}

func (gs *GuildState) cancelInactivityTimer() {
	if gs.inactiveTimer != nil {
		gs.inactiveTimer.Stop()
		gs.inactiveTimer = nil
	}
}
