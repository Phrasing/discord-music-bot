# djvon
A simple Discord music bot with an AI DJ mode that plays audio from YouTube.
<img width="472" alt="REEPUQPn" src="https://github.com/user-attachments/assets/c7084dae-b4a1-4ded-955e-f4fe3e509d64" width="" height=""/>
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
    git clone https://github.com/Phrasing/djvon.git
    cd djvon
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

    It is also highly recommended to use the nightly version of `yt-dlp` to keep up with YouTube's changes. To enable support for impersonating browser requests, which can help with sites that use TLS fingerprinting, install `yt-dlp` with the `curl-cffi` extra:

    ```bash
    pipx install --pip-args=--pre "yt-dlp[default,curl-cffi]"
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

3.  **Set up YouTube Cookies:**

    To bypass YouTube's bot detection, you need to provide a `cookies.txt` file. You can start by copying the example file:

    ```bash
    cp cookies.txt.example cookies.txt
    ```

    In many cases, `yt-dlp` can automatically generate the necessary cookies. However, if you encounter issues, you may need to provide your own cookies by following these steps:

    -   Install a browser extension that can export cookies in the Netscape format, such as [Cookie Editor](https://chromewebstore.google.com/detail/cookie-editor/hlkenndednhfkekhgcdicdfddnkalmdm) for Chrome.
    -   Go to [YouTube](https://www.youtube.com) and make sure you are logged in.
    -   Use the extension to export your cookies for the `youtube.com` domain and overwrite the contents of `cookies.txt`.

    Finally, update your `.env` file with the path to the cookies file:

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

6.  **Set up Gemini API (Optional):**

    -   Go to the [Google AI Studio](https://aistudio.google.com/app/apikey) to get your API key.
    -   Add it to your `.env` file:

    ```
    GEMINI_API_KEY=YOUR_GEMINI_API_KEY
    ```

7.  **Set up a Custom DJ Prompt (Optional):**

    You can customize the prompt used by the `/dj` command by creating a text file and setting the `DJ_PROMPT_FILE_PATH` in your `.env` file. The default prompt can be found in `djprompt.txt`.

    ```
    DJ_PROMPT_FILE_PATH=path/to/your/prompt.txt
    ```

## Running the Bot

### With Docker

You can run the bot using the pre-built Docker image from the GitHub Container Registry.

1.  **Log in to the GitHub Container Registry:**

    ```bash
    echo ${{ secrets.GITHUB_TOKEN }} | docker login ghcr.io -u ${{ github.actor }} --password-stdin
    ```
    *Note: You will need to create a Personal Access Token (PAT) with the `read:packages` scope and use it in place of `${{ secrets.GITHUB_TOKEN }}` if you are running this locally.*

2.  **Pull the latest image:**

    ```bash
    docker pull ghcr.io/phrasing/djvon:latest
    ```

3.  **Run the container:**

    ```bash
    docker run -d --env-file .env --restart unless-stopped --name djvon ghcr.io/phrasing/djvon:latest
    ```

Alternatively, you can build the image locally:

```bash
docker build -t djvon:latest .
docker run -d --env-file .env --restart unless-stopped --name djvon djvon:latest
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
-   `/dj <genre>`: Let the AI DJ play a set for you based on a genre.

You can also use the buttons on the "Now Playing" message to control the music.
