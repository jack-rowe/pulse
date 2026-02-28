# Contributing to Pulse

## Development Setup

```bash
git clone https://github.com/jack-rowe/pulse.git
cd pulse
go mod tidy
make build
```

## Running Locally

```bash
# Generate config
./pulse --init

# Edit config.yaml, then:
./pulse --config config.yaml --debug
```

## Testing

```bash
make test
```

## Project Structure

```
├── main.go              # Entry point, CLI flags, wiring
├── config/              # YAML config loading + validation
├── checker/             # Health check implementations (HTTP, TCP, WebSocket)
├── scheduler/           # Goroutine-per-target check orchestration
├── store/               # Persistence (BBolt embedded DB)
├── notifier/            # Alert channels (Slack, Discord, SMTP, webhook)
├── api/                 # HTTP API + embedded status page
├── Makefile             # Build, test, release targets
├── Dockerfile           # Multi-stage Docker build
└── pulse.service        # Systemd unit file
```

## Pull Requests

1. Fork the repo and create a branch from `main`
2. Add tests for new functionality
3. Run `make test && make lint` before submitting
4. Keep PRs focused — one feature or fix per PR
5. Write clear commit messages

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Use `log/slog` for all logging (no `fmt.Println` in library code)
- All public types and functions need doc comments
- Interfaces go in their own file (e.g., `store.go` for the interface, `bolt.go` for the implementation)

## Reporting Issues

Use GitHub Issues. Include:

- Pulse version (`pulse --version`)
- OS and architecture
- Config (redact secrets)
- Full error output
