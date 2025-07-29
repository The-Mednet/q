# SMTP Relay Service - Development Guide

## Project Overview
This is an SMTP relay service that acts as a drop-in replacement for Mandrill, with Google Workspace integration and optional LLM-powered email personalization. Built for Mednet's email infrastructure needs.

## Technology Stack
- **Language**: Go 1.23
- **Database**: MySQL (primary), Redis (potential), DynamoDB (potential)
- **Email**: Gmail API via Google Workspace OAuth
- **LLM**: OpenAI/Azure OpenAI/Groq for email personalization
- **Monitoring**: New Relic integration (via blaster module)
- **Queue**: MySQL-backed with in-memory fallback
- **Web UI**: Vanilla JS with REST API

## Architecture
- **SMTP Server**: Port 2525, handles incoming emails
- **Queue System**: MySQL persistence with rate limiting (2000/day)
- **Processor**: Background worker for email sending
- **Web UI**: Management dashboard on port 8080
- **Webhooks**: Mandrill-compatible event notifications

## Key Dependencies
- `blaster` - Internal Mednet module (local replacement at `/Users/bea/dev/mednet/blaster`)
- `github.com/emersion/go-smtp` - SMTP server implementation
- `github.com/go-sql-driver/mysql` - MySQL driver
- `google.golang.org/api` - Gmail API client
- `golang.org/x/oauth2` - OAuth2 flow

## Development Commands

### Build & Run
```bash
make build          # Build binary
make run            # Run directly with go run
make test           # Run all tests
make clean          # Clean build artifacts
```

### Linting & Formatting
```bash
make lint           # Run golangci-lint
make fmt            # Format code with go fmt
```

### Docker Development
```bash
make docker-build   # Build Docker image
make docker-up      # Start with docker-compose
make docker-down    # Stop docker-compose
make docker-logs    # View logs
make docker-reset   # Reset everything including volumes
make dev            # Development mode with hot reload
```

### Database
```bash
make db-init        # Initialize MySQL schema
```

### Setup
```bash
make setup          # Full local setup (deps + db + env)
make docker-setup   # Docker setup
```

## Configuration
- Uses `.env` file for configuration (following Mednet standard)
- OAuth credentials in `credentials/` directory
- Supports both local MySQL and Docker MySQL
- Rate limiting enforced for Gmail API (2000 emails/24h)

## Testing Strategy
- Run `go test ./...` for unit tests
- Integration tests require MySQL and Gmail credentials
- Use `make test` for comprehensive testing
- Docker environment for isolated testing

## Production Considerations
- Designed for high reliability (serves doctors)
- Defensive programming practices implemented
- Error handling with retries and fallbacks
- MySQL queue persistence for reliability
- Rate limiting to respect Gmail quotas
- Monitoring via New Relic (through blaster module)

## File Structure
```
cmd/server/          # Main application entry
internal/
  ├── config/        # Configuration management  
  ├── gmail/         # Google Workspace integration
  ├── llm/           # LLM personalization
  ├── processor/     # Queue processing logic
  ├── queue/         # Queue implementations (MySQL/memory)
  ├── smtp/          # SMTP server
  ├── webhook/       # Mandrill webhook compatibility
  └── webui/         # Web UI server
pkg/models/          # Shared data models
static/              # Web UI assets (JS/CSS)
schema.sql           # MySQL database schema
```

## Git Integration
- Project is not currently in git - consider initializing with `git init`
- Should be added to https://github.com/The-Mednet organization
- Follow Mednet's git workflow and commit standards

## Notes
- No git repository initialized yet
- Uses local `blaster` module dependency
- MySQL tables need to be initialized before first run
- OAuth flow required for Gmail integration
- LLM features are optional but add significant value