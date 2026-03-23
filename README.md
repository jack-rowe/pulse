# Pulse

Single-binary uptime monitor. Zero dependencies. Self-hosted.

Monitor HTTP endpoints, TCP ports, and WebSocket connections. Get alerts via Slack, Discord, email, or webhooks. View status on a built-in dashboard.

## Quick Start

```bash
# Download (Linux amd64)
curl -Lo pulse https://github.com/jack-rowe/pulse/releases/latest/download/pulse-linux-amd64
chmod +x pulse

# Generate config
./pulse --init

# Edit config.yaml with your endpoints, then:
./pulse --config config.yaml
```

Open `http://localhost:8080` for the status dashboard.

## Features

- **Single binary** — no runtime, no containers, no dependencies
- **HTTP/TCP/WebSocket** health checks with configurable intervals
- **Embedded status page** — dark-mode dashboard at `/`
- **JSON API** — `/api/status` for integrations
- **Slack, Discord, email, webhook** alerting on state changes
- **Failure debouncing** — configurable threshold before alerting (no false alarms)
- **Embedded storage** — BBolt key-value store, single file, zero setup
- **Auto-cleanup** — configurable data retention
- **Cross-platform** — Linux, macOS, Windows, ARM64

## Configuration

```yaml
server:
  port: 8080
  # api_key: "secret"  # Optional: protect API endpoints

endpoints:
  - name: "Production API"
    type: http
    url: "https://api.example.com/health"
    expected_status: 200
    timeout_ms: 5000
    interval_sec: 30
    fail_threshold: 3

  - name: "Website"
    type: http
    url: "https://www.example.com"
    expected_status: 200
    timeout_ms: 5000
    interval_sec: 60

  - name: "Realtime Server"
    type: websocket
    url: "wss://rt.example.com/ws"
    timeout_ms: 5000
    interval_sec: 15

  - name: "Database"
    type: tcp
    address: "db.internal:5432"
    timeout_ms: 3000
    interval_sec: 30

alerting:
  slack:
    webhook_url: "https://hooks.slack.com/services/..."
  # discord:
  #   webhook_url: "https://discord.com/api/webhooks/..."
  # smtp:
  #   host: smtp.gmail.com
  #   port: 587
  #   username: alerts@example.com
  #   password: app-password
  #   from: alerts@example.com
  #   to: [oncall@example.com]

storage:
  path: "pulse.db"
  retention_days: 90
```

## API

| Endpoint                  | Description                              |
| ------------------------- | ---------------------------------------- |
| `GET /`                   | Status page (HTML)                       |
| `GET /health`             | Self health check                        |
| `GET /api/status`         | All endpoints — current state + uptime % |
| `GET /api/status/{name}`  | Single endpoint detail                   |
| `GET /api/history/{name}` | Recent check history                     |

If `api_key` is set in config, pass it as `X-API-Key` header or `?api_key=` query param.
For the built-in dashboard with auth enabled, open `/?api_key=your-secret` in your browser.

## CLI

```
pulse [flags]

Flags:
  --config string   Path to config file (default "config.yaml")
  --init            Generate default config.yaml
  --validate        Validate config and exit
  --version         Print version
  --debug           Enable debug logging
```

## Deploy

### Systemd (Linux)

```bash
# Copy binary
sudo cp pulse /usr/local/bin/
sudo chmod +x /usr/local/bin/pulse

# Create user + dirs
sudo useradd -r -s /bin/false pulse
sudo mkdir -p /etc/pulse /var/lib/pulse
sudo cp config.yaml /etc/pulse/
sudo chown -R pulse:pulse /var/lib/pulse

# Install service
sudo cp pulse.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now pulse
```

### Docker

```bash
docker run -d \
  -v $(pwd)/config.yaml:/config.yaml \
  -p 8080:8080 \
  pulse:latest
```

### Docker Compose

```bash
# Generate config.yaml in this directory
docker compose run --rm pulse --init

# Edit config.yaml, then start
docker compose up -d
```

## Build from Source

```bash
git clone https://github.com/jack-rowe/pulse.git
cd pulse
make build
```

## License

AGPL-3.0 — See [LICENSE](LICENSE).
