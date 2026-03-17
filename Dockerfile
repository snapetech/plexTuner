FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -mod=vendor -o /iptv-tunerr ./cmd/iptv-tunerr

FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl wget ffmpeg
COPY --from=build /iptv-tunerr /usr/local/bin/iptv-tunerr
EXPOSE 5004
ENTRYPOINT ["iptv-tunerr"]
# Default: run tuner (refresh catalog, health check, serve). Override: e.g. docker run ... iptv-tunerr:local serve -addr :5004
CMD ["run", "-addr", ":5004"]
