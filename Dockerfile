# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo docker)" -o pulse .

# Runtime stage — scratch = no OS, just the binary
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/pulse /pulse
EXPOSE 8080
ENTRYPOINT ["/pulse"]
CMD ["--config", "/config.yaml"]
