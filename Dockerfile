# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Install build dependencies
RUN apk add --no-cache git build-base ffmpeg

# Install dca
RUN go install github.com/bwmarrin/dca/cmd/dca@latest

# Copy go modules and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code and build the application
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/bot .

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ffmpeg python3 py3-pip
RUN pip3 install --no-cache-dir yt-dlp

WORKDIR /app

# Copy the compiled bot binary and dca from the builder stage
COPY --from=builder /app/bot .
COPY --from=builder /go/bin/dca /usr/local/bin/

# Run the bot
CMD ["./bot"]
