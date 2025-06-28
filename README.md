# Discord Music Bot

A simple Discord music bot that plays audio from YouTube.

## Features

- Plays audio from YouTube, SoundCloud, and Spotify.
- Supports queueing songs.
- Automatically disconnects after 30 seconds of inactivity.
- Uses slash commands for interaction.
- Automatically checks for `yt-dlp` updates every 24 hours to ensure reliability.

## Prerequisites

- [Go](https://golang.org/doc/install) (version 1.18 or higher)
- [yt-dlp](https://github.com/yt-dlp/yt-dlp)
- [ffmpeg](https://ffmpeg.org/download.html)
- C libraries for Opus:
  - **Debian/Ubuntu**: `libopus-dev`, `libopusfile-dev`
  - **Mac (Homebrew)**: `opus`, `opusfile`

## Installation

1.  **Clone the repository:**

    ```bash
    git clone https://github.com/Phrasing/discord-music-bot.git
    cd discord-music-bot
    ```

2.  **Install Go dependencies:**

    ```bash
    go mod tidy
    ```

3.  **Install System Dependencies:**

    Install `yt-dlp`, `ffmpeg`, and the Opus C libraries using your system's package manager.

    **For Debian/Ubuntu:**
    ```bash
    sudo apt-get update && sudo apt-get install -y ffmpeg libopus-dev libopusfile-dev
    ```

    **For Mac (Homebrew):**
    ```bash
    brew install ffmpeg opus opusfile
    ```

    It is also highly recommended to use the nightly version of `yt-dlp` to keep up with YouTube's changes. You can install it using `pipx`:

    ```bash
    pipx install --pip-args=--pre "yt-dlp[default]"
    pipx ensurepath
    ```

## Configuration

1.  **Create a Discord Bot:**

    - Go to the [Discord Developer Portal](https://discord.com/developers/applications).
    - Click "New Application".
    - Give your application a name and click "Create".
    - Go to the "Bot" tab and click "Add Bot".
    - Copy the bot's token.

2.  **Create a `.env` file:**

    Create a file named `.env` in the root of the project and add your bot token:

    ```
    BOT_TOKEN=YOUR_BOT_TOKEN
    ```

3.  **Get YouTube Cookies:**

    To bypass YouTube's bot detection, you need to provide a `cookies.txt` file.

    -   Install a browser extension that can export cookies in the Netscape format, such as [Cookie Editor](https://chromewebstore.google.com/detail/cookie-editor/hlkenndednhfkekhgcdicdfddnkalmdm) for Chrome.
    -   Go to [YouTube](https://www.youtube.com) and make sure you are logged in.
    -   Use the extension to export your cookies for the `youtube.com` domain.
    -   Save the exported cookies as `cookies.txt` in the root of the project.

    Update your `.env` file with the path to the cookies file:

    ```
    COOKIES_PATH=cookies.txt
    ```

4.  **Set up Spotify (Optional):**

    -   Go to the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard/).
    -   Click "Create an App".
    -   Give your application a name and description and click "Create".
    -   Copy the Client ID and Client Secret.
    -   Add them to your `.env` file:

    ```
    SPOTIFY_CLIENT_ID=YOUR_CLIENT_ID
    SPOTIFY_CLIENT_SECRET=YOUR_CLIENT_SECRET
    ```

5.  **Using a Proxy for `yt-dlp` (Optional):**

    If you need to use a proxy for `yt-dlp`, you can set the `YT_DLP_PROXY` environment variable in your `.env` file:

    ```
    YT_DLP_PROXY=YOUR_PROXY_URL
    ```

## Running the Bot

### With Docker

To run the bot using Docker, you can use the following command:

```bash
docker run -d --env-file .env --restart unless-stopped --name discord-music-bot ghcr.io/Phrasing/discord-music-bot:latest
```

### Without Docker

```bash
go run .
```

## Commands

-   `/play <url_or_search_query>`: Plays a song from a YouTube URL, Spotify URL, SoundCloud URL, or a search query. Adds the song to the queue if one is already playing.
-   `/stop`: Stops the music, clears the queue, and disconnects the bot from the voice channel.
-   `/skip`: Skips the current song and plays the next one in the queue.
-   `/pause`: Pauses or resumes the current song.

You can also use the buttons on the "Now Playing" message to control the music.
