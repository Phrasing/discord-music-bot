package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/zmb3/spotify"
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

	if state.skipChan != nil {
		select {
		case state.skipChan <- true:
			respondEphemeral(s, i, "Skipped the current song")
		default:
			respondEphemeral(s, i, "Nothing to skip")
		}
	} else {
		respondEphemeral(s, i, "Nothing to skip")
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

	// Clear queue
	for !state.queue.IsEmpty() {
		state.queue.Get()
	}

	// Kill process if running
	if state.process != nil {
		state.process.Kill()
	}

	// Update now playing message
	if state.nowPlaying != nil {
		newContent := "Playback stopped."
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Content:    &newContent,
			Components: &[]discordgo.MessageComponent{},
			ID:         state.nowPlaying.ID,
			Channel:    state.nowPlaying.ChannelID,
		})
		state.nowPlaying = nil
	}

	// Disconnect
	b.disconnectFromGuild(i.GuildID)
	respondEphemeral(s, i, "Stopped playing and left the voice channel")
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
	if state.skipChan != nil {
		select {
		case state.skipChan <- true:
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredMessageUpdate,
			})
		default:
		}
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

func resolveSpotifyURL(url string) (string, error) {
	parts := strings.Split(url, "track/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid Spotify URL")
	}

	trackIDString := parts[1]
	trackID := spotify.ID(strings.Split(trackIDString, "?")[0])

	trackName, err := getTrackName(trackID)
	if err != nil {
		return "", fmt.Errorf("getting track name: %w", err)
	}

	return searchYoutube(trackName)
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
	respondEphemeral(s, i, "Processing...")

	switch i.ApplicationCommandData().Name {
	case "play":
		b.handlePlay(s, i)
	case "skip":
		b.handleSkip(s, i)
	case "pause":
		b.handlePause(s, i)
	case "stop":
		b.handleStop(s, i)
	}
}

func (b *Bot) handlePlay(s *discordgo.Session, i *discordgo.InteractionCreate) {
	query := i.ApplicationCommandData().Options[0].StringValue()

	voiceChannelID := getUserVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if voiceChannelID == "" {
		editResponse(s, i, "You must be in a voice channel")
		return
	}

	videoURL, err := resolveURL(query)
	if err != nil {
		editResponse(s, i, fmt.Sprintf("Error: %v", err))
		return
	}

	info, err := getVideoInfo(videoURL)
	if err != nil {
		editResponse(s, i, "Error getting video info")
		return
	}

	state := b.getOrCreateGuildState(i.GuildID)

	if err := b.ensureVoiceConnection(s, i.GuildID, voiceChannelID, state); err != nil {
		editResponse(s, i, "Error joining voice channel")
		return
	}

	song := &Song{
		URL:       info.URL,
		Title:     info.Title,
		Duration:  info.Duration,
		ChannelID: i.ChannelID,
	}

	state.queue.Add(song)

	if state.voice != nil && len(state.voice.OpusSend) == 0 {
		editResponse(s, i, fmt.Sprintf("Playing: %s", info.Title))
		go b.playNext(s, i.GuildID)
	} else {
		editResponse(s, i, fmt.Sprintf("Added to queue: %s", info.Title))
	}
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

func (b *Bot) playNext(s *discordgo.Session, guildID string) {
	state := b.getOrCreateGuildState(guildID)

	song := state.queue.Get()
	if song == nil {
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

	content := fmt.Sprintf("Now playing: %s", song.URL)
	if song.Duration > 0 {
		content += fmt.Sprintf("\n`00:00 / %s`", formatDuration(song.Duration))
	}

	msg, err := s.ChannelMessageSendComplex(song.ChannelID, &discordgo.MessageSend{
		Content:    content,
		Components: components,
	})
	if err == nil {
		state.nowPlaying = msg
	}

	streamURL, err := getStreamURL(song.URL, config)
	if err != nil {
		log.Printf("Error getting stream URL: %v", err)
		s.ChannelMessageSend(song.ChannelID, "Error getting audio stream.")
		b.playNext(s, guildID)
		return
	}

	ffmpeg := exec.Command("ffmpeg",
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "5",
		"-nostdin",
		"-i", streamURL,
		"-f", "s16le",
		"-ar", "48000",
		"-ac", "2",
		"pipe:1")

	ffmpegErr, err := ffmpeg.StderrPipe()
	if err != nil {
		log.Printf("Error getting ffmpeg stderr pipe: %v", err)
		b.playNext(s, guildID)
		return
	}

	ffmpegOut, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Printf("Error getting ffmpeg stdout pipe: %v", err)
		b.playNext(s, guildID)
		return
	}

	go func() {
		scanner := bufio.NewScanner(ffmpegErr)
		for scanner.Scan() {
			log.Printf("[ffmpeg] %s", scanner.Text())
		}
	}()

	if err := ffmpeg.Start(); err != nil {
		log.Printf("Error starting ffmpeg: %v", err)
		b.playNext(s, guildID)
		return
	}
	log.Println("ffmpeg started")

	state.mu.Lock()
	state.process = ffmpeg.Process
	state.skipChan = make(chan bool)
	state.mu.Unlock()

	done := make(chan bool)
	go func() {
		b.updateNowPlaying(s, state, song, done)
	}()

	b.streamAudio(state.voice, ffmpegOut, state, config)

	close(done)
	ffmpeg.Wait()

	state.mu.Lock()
	state.process = nil
	if state.skipChan != nil {
		close(state.skipChan)
		state.skipChan = nil
	}
	state.mu.Unlock()

	log.Println("playSound finished")

	b.playNext(s, guildID)
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

func (b *Bot) streamAudio(vc *discordgo.VoiceConnection, audio io.Reader, state *GuildState, config *Config) {
	const (
		channels  = 2
		frameRate = 48000
		frameSize = 960
		maxBytes  = (frameSize * channels * 2)
	)

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
			if state.paused {
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

func resolveURL(query string) (string, error) {
	if strings.Contains(query, "spotify.com") {
		return resolveSpotifyURL(query)
	}
	if strings.HasPrefix(query, "http") {
		return query, nil
	}
	return searchYoutube(query)
}

func getVideoInfo(url string) (*VideoInfo, error) {
	cmd := exec.Command("yt-dlp",
		"--dump-json",
		"--no-playlist",
		url)

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var data struct {
		Title    string `json:"title"`
		Duration int    `json:"duration"`
	}

	if err := json.Unmarshal(output, &data); err != nil {
		return nil, err
	}

	return &VideoInfo{
		URL:      url,
		Title:    data.Title,
		Duration: time.Duration(data.Duration) * time.Second,
	}, nil
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
