# Example Dockerfile - customize for your project
# This is a template - replace with your application's build steps

# --- Build stage ---
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Replace ./cmd/your-app with your actual main package path
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /your-app ./cmd/your-app

# --- Runtime stage ---
FROM debian:stable-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /your-app /usr/local/bin/your-app
EXPOSE 8080
ENTRYPOINT ["your-app"]
CMD ["--help"]
