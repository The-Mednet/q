# Blaster Module Configuration

## Current Setup (Local Development)
The project currently uses a local path for the `blaster` module during development:

```go
// go.mod
require (
    github.com/The-Mednet/blaster v0.0.0
    // ... other dependencies
)

// Temporary local development
replace github.com/The-Mednet/blaster => /Users/bea/dev/mednet/blaster
```

## Switching to GitHub

### Prerequisites
1. SSH key configured for GitHub access
2. Access to the private `The-Mednet/blaster` repository

### Steps to Switch

1. **Configure Go for private repositories:**
```bash
go env -w GOPRIVATE=github.com/The-Mednet
```

2. **Configure Git to use SSH:**
```bash
git config --global url."git@github.com:".insteadOf "https://github.com/"
```

3. **Remove the replace directive from go.mod:**
```go
// Remove this line:
replace github.com/The-Mednet/blaster => /Users/bea/dev/mednet/blaster
```

4. **Get the latest version:**
```bash
go get github.com/The-Mednet/blaster@latest
```

5. **Tidy up dependencies:**
```bash
go mod tidy
```

## Import Paths
All imports have been updated to use the GitHub path:
```go
import (
    blasterLLM "github.com/The-Mednet/blaster/pkg/llm"
    "github.com/The-Mednet/blaster/pkg/types"
)
```

## Troubleshooting

### Permission Denied Error
If you see:
```
git@github.com: Permission denied (publickey)
```

Ensure:
- Your SSH key is added to GitHub
- You have access to the private repository
- SSH agent is running: `ssh-add -l`

### Module Not Found
If the module can't be found:
- Verify the repository exists at `github.com/The-Mednet/blaster`
- Check that it has a proper `go.mod` file
- Ensure you have read access to the repository

## Development Workflow

### For Local Development
Keep the replace directive in go.mod for faster iteration:
```go
replace github.com/The-Mednet/blaster => /Users/bea/dev/mednet/blaster
```

### For Production/CI
Remove the replace directive and use the GitHub repository directly.