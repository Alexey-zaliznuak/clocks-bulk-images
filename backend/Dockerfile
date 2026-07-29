# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# cache deps
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

# ---- runtime stage ----
FROM alpine:3.20
# ffmpeg/ffprobe fit generated clips onto soundtracks and extract mp3 tracks.
RUN apk add --no-cache ca-certificates tzdata ffmpeg && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/server /app/server
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
