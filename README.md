# Discord Music Bot

A simple Discord music bot that plays audio from YouTube.

## Features

- Plays audio from YouTube links.
- Supports queueing songs.
- Automatically disconnects after 30 seconds of inactivity.
- Uses slash commands for interaction.

## Prerequisites

- [Go](https://golang.org/doc/install) (version 1.18 or higher)
- [yt-dlp](https://github.com/yt-dlp/yt-dlp)
- [ffmpeg](https://ffmpeg.org/download.html)
- [dca](https://github.com/bwmarrin/dca)

## Installation

1.  **Clone the repository:**

    ```bash
    git clone https://github.com/Phrasing/yt-discord-music-bot.git
    cd yt-discord-music-bot
    ```

2.  **Install Go dependencies:**

    ```bash
    go mod tidy
    ```

3.  **Install `yt-dlp`, `ffmpeg`, and `dca`:**

    Follow the installation instructions for your operating system from the links in the Prerequisites section.

    It is highly recommended to use the nightly version of `yt-dlp` to keep up with YouTube's changes. You can install it using `pipx`:

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

## Running the Bot

```bash
go run .
```

## Commands

-   `/play <youtube_url>`: Plays a song from the given YouTube URL or adds it to the queue if a song is already playing.
-   `/stop`: Stops the music, clears the queue, and disconnects the bot from the voice channel.
