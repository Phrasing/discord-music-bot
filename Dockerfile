# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Install build dependencies
RUN apk add --no-cache git build-base ffmpeg opus-dev opusfile-dev

# Copy go modules and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code and build the application
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /app/bot .

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ffmpeg python3 py3-pip opus opusfile pipx
RUN pipx install --pip-args=--pre "yt-dlp[default,curl-cffi]"
ENV PATH="/root/.local/bin:$PATH"

WORKDIR /app

# Copy the compiled bot binary from the builder stage
COPY --from=builder /app/bot .

# Run the bot
CMD ["./bot"]
