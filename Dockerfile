FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 go build -o /app ./cmd/hello

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /app /usr/local/bin/hello
ENTRYPOINT ["hello"]
