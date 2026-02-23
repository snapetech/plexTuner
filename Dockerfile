FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /plex-tuner ./cmd/plex-tuner

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
# For HLS materializer (mount with cache): add ffmpeg: apk add ffmpeg
COPY --from=build /plex-tuner /usr/local/bin/plex-tuner
EXPOSE 5004
ENTRYPOINT ["plex-tuner"]
