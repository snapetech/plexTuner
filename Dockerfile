FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /plex-tuner ./cmd/plex-tuner

FROM debian:stable-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /plex-tuner /usr/local/bin/plex-tuner
EXPOSE 5004
ENTRYPOINT ["plex-tuner"]
# Default: run tuner (refresh catalog, health check, serve). Override: e.g. docker run ... plex-tuner:local serve -addr :5004
CMD ["run", "-addr", ":5004"]
