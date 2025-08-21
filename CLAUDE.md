# SMTP Relay Service - Development Guide

## Project Overview
This is an SMTP relay service that acts as a drop-in replacement for Mandrill, with unified multi-provider support for Gmail and Mailgun, plus optional LLM-powered email personalization. Built for Mednet's email infrastructure needs.

## Technology Stack
- **Language**: Go 1.23
- **Database**: MySQL (primary), Redis (potential), DynamoDB (potential)
- **Email Providers**: Gmail API (service account), Mailgun API
- **LLM**: OpenAI/Azure OpenAI/Groq for email personalization
- **Monitoring**: New Relic integration (via blaster module)
- **Queue**: MySQL-backed with unified rate limiting
- **Web UI**: Vanilla JS with REST API

## Architecture
- **SMTP Server**: Port 2525, handles incoming emails
- **Unified Provider System**: Gmail and Mailgun providers with shared interfaces
- **Workspace Router**: Domain-based routing to appropriate providers
- **Queue System**: MySQL persistence with per-workspace rate limiting
- **Processor**: Background worker supporting multiple providers
- **Web UI**: Management dashboard on port 8080
- **Webhooks**: Mandrill-compatible event notifications
- **Recipient Tracking**: MySQL-based delivery tracking system

## Key Dependencies
- `blaster` - Internal Mednet module (local replacement at `/Users/bea/dev/mednet/blaster`)
- `github.com/emersion/go-smtp` - SMTP server implementation
- `github.com/go-sql-driver/mysql` - MySQL driver
- `google.golang.org/api` - Gmail API client
- `golang.org/x/oauth2` - OAuth2 flow for Gmail service accounts
- `github.com/mailgun/mailgun-go/v4` - Mailgun API client

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

### Workspace Configuration
The service uses a unified workspace configuration system supporting multiple email providers:

#### File-based Configuration (`workspace.json`)
```json
[
  {
    "id": "gmail-workspace",
    "domain": "yourdomain.com",
    "display_name": "Gmail Workspace",
    "rate_limits": {
      "workspace_daily": 2000,
      "per_user_daily": 100,
      "custom_user_limits": {
        "vip@yourdomain.com": 5000
      }
    },
    "gmail": {
      "service_account_file": "credentials/service-account.json",
      "enabled": true,
      "default_sender": "noreply@yourdomain.com"
    }
  },
  {
    "id": "mailgun-workspace", 
    "domain": "mail.yourdomain.com",
    "display_name": "Mailgun Workspace",
    "rate_limits": {
      "workspace_daily": 10000,
      "per_user_daily": 500
    },
    "mailgun": {
      "api_key": "your-mailgun-api-key",
      "domain": "mail.yourdomain.com",
      "base_url": "https://api.mailgun.net/v3",
      "enabled": true,
      "tracking": {
        "opens": true,
        "clicks": true
      }
    }
  }
]
```

#### Environment Variables
- Uses `.env` file for configuration (following Mednet standard)
- Service account credentials in `credentials/` directory  
- Supports both local MySQL and Docker MySQL
- Production: `WORKSPACES_JSON` environment variable for AWS Secrets Manager integration

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
  ├── config/        # Configuration management & workspace loading
  ├── provider/      # Unified provider system (Gmail, Mailgun)
  ├── workspace/     # Workspace management and routing  
  ├── recipient/     # Recipient tracking system
  ├── gateway/       # Legacy gateway integration (migration)
  ├── llm/           # LLM personalization
  ├── processor/     # Unified queue processing logic
  ├── queue/         # Queue implementations (MySQL/memory)
  ├── smtp/          # SMTP server
  ├── webhook/       # Mandrill webhook compatibility
  └── webui/         # Web UI server
pkg/models/          # Shared data models
static/              # Web UI assets (JS/CSS)
schema.sql           # MySQL database schema
workspace.json       # Workspace configuration
```

## Git Integration
- Project uses git with recent commits including unified provider architecture
- Should be added to https://github.com/The-Mednet organization
- Follow Mednet's git workflow and commit standards

## Provider System

### Gmail Provider
- Uses Google Workspace service account authentication  
- Supports domain-wide delegation for impersonation
- Rate limited to 2000 emails/24h per workspace
- Requires `gmail.send` scope
- Health checks validate OAuth2 token generation

### Mailgun Provider  
- Uses Mailgun API with domain verification
- Supports tracking for opens, clicks, unsubscribes
- Higher rate limits than Gmail
- Built-in domain rewriting for flexible sender addresses

### Workspace Routing
- Automatic provider selection based on sender domain
- Domain mapping: `user@domain.com` → appropriate workspace
- Unified rate limiting across all providers in workspace
- Per-user rate limits with custom overrides

## Key Features
- **Multi-Provider**: Unified interface for Gmail and Mailgun
- **Domain Routing**: Automatic workspace selection
- **Rate Limiting**: Per-workspace and per-user limits
- **Health Monitoring**: Real-time provider status
- **Recipient Tracking**: MySQL-based delivery tracking
- **Environment Config**: AWS Secrets Manager integration
- **Defensive Programming**: Comprehensive error handling
- **LLM Integration**: Optional email personalization