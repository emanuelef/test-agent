# Weather Wind Forecast Agent

A Go-based agent that fetches wind forecast data for London Heathrow Airport and uses Ollama for AI-powered analysis.

## Features

- Fetches 15-day wind forecast from Open-Meteo API (no API key required - completely free)
- Analyzes wind patterns using local Ollama LLM
- Provides actionable insights for airport operations
- Containerized for easy deployment

## Prerequisites

- Go 1.25 or later (for local development)
- Docker (for containerized deployment)
- Ollama running locally on port 11434

## Configuration

Configure via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `OLLAMA_HOST` | `http://127.0.0.1:11434` | Ollama API endpoint |
| `OLLAMA_MODEL` | `gemma2:9b` | Ollama model to use |
| `FORECAST_DAYS` | `15` | Number of forecast days (max 16) |

## Environment Variables

Copy `.env.example` to `.env` and fill in your secrets and configuration. The `.env` file is ignored by git and should not be committed.

```bash
cp .env.example .env
# Edit .env and set your values
```

## Telegram Integration

To receive the Ollama summary via Telegram, set the following environment variables:

- `TELEGRAM_TOKEN`: Your Telegram bot token
- `TELEGRAM_CHAT_ID`: The chat ID to send messages to

### How to get your Telegram Bot Token and Chat ID

1. **Create a Telegram Bot:**
   - Open Telegram and search for [@BotFather](https://t.me/BotFather)
   - Start a chat and send `/newbot`
   - Follow the instructions to set a name and username
   - BotFather will reply with your **bot token** (save this for `TELEGRAM_TOKEN`)

2. **Get your Chat ID:**
   - Add your new bot to a group (if you want group notifications) or start a chat with it
   - Send a message to the bot (or in the group)
   - Visit: `https://api.telegram.org/bot<YOUR_BOT_TOKEN>/getUpdates`
   - Look for `chat":{"id":...` in the JSON response; this is your **chat ID** (use for `TELEGRAM_CHAT_ID`)

3. **Set these in your `.env` file:**
   - `TELEGRAM_TOKEN=...`
   - `TELEGRAM_CHAT_ID=...`

### How to get your Telegram Chat ID

1. Start a chat with your bot in Telegram and send any message (e.g., "Hi").
2. Run:
   ```
   curl "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/getUpdates"
   ```
3. In the JSON response, look for `"chat":{"id":...}`. For example:
   ```json
   {"ok":true,"result":[{"update_id":879971365,
   "message":{"message_id":2,"from":{"id":8322824979,"is_bot":false,"first_name":"Emanuele","last_name":"Fumagalli","language_code":"it"},"chat":{"id":8322824979,"first_name":"Emanuele","last_name":"Fumagalli","type":"private"},"date":1769756335,"text":"Hi"}}]}
   ```
   The `id` field (e.g., `8322824979`) is your `TELEGRAM_CHAT_ID`.

You can use a `.env` file for convenience. Example:

```env
OLLAMA_HOST=http://127.0.0.1:11434
OLLAMA_MODEL=llama3.2:3b
FORECAST_DAYS=15
TELEGRAM_TOKEN=your_telegram_bot_token
TELEGRAM_CHAT_ID=your_telegram_chat_id
```

To run with Docker and .env:

```bash
docker run --rm --network host --env-file .env ghcr.io/emanuelef/test-agent:latest
```

Or set variables directly:

```bash
docker run --rm --network host \
  -e TELEGRAM_TOKEN=your_telegram_bot_token \
  -e TELEGRAM_CHAT_ID=your_telegram_chat_id \
  -e OLLAMA_MODEL=llama3.2:3b \
  ghcr.io/emanuelef/test-agent:latest
```

## Local Development

```bash
# Run directly
go run ./cmd/agent

# Build binary
go build -o agent ./cmd/agent
./agent

# With custom settings
FORECAST_DAYS=10 OLLAMA_MODEL=llama2 go run ./cmd/agent
```

## Docker Deployment

### Build locally

```bash
docker build -t weather-agent .
```

### Run container with Ollama on host

When Ollama is installed directly on the host machine (not in Docker):

```bash
# On Linux (e.g., Oracle VM)
docker run --rm --network host weather-agent

# Alternative on Linux - explicitly set host
docker run --rm \
  --add-host=host.docker.internal:host-gateway \
  -e OLLAMA_HOST=http://host.docker.internal:11434 \
  weather-agent

# On macOS/Windows (use host.docker.internal)
docker run --rm \
  -e OLLAMA_HOST=http://host.docker.internal:11434 \
  weather-agent
```

**For Oracle Cloud VM with Ollama installed on host:**
The simplest approach is to use `--network host`, which allows the container to access services on the host's localhost:

```bash
docker run --rm --network host weather-agent
```

Or build and run using Make:
```bash
make docker-build
docker run --rm --network host weather-agent
```

### Docker Compose (Ollama in Docker)

If running Ollama in a container:

```yaml
version: '3.8'
services:
  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"
    volumes:
      - ollama-data:/root/.ollama
    
  weather-agent:
    image: ghcr.io/emanuelefumagalli/test-agent:latest
    depends_on:
      - ollama
    environment:
      - OLLAMA_HOST=http://ollama:11434
      - OLLAMA_MODEL=llama3.1
      - FORECAST_DAYS=15

volumes:
  ollama-data:
```

## CI/CD

GitHub Actions workflow automatically builds and pushes Docker images to GitHub Container Registry on:
- Push to main/master branch
- Tagged releases
- Pull requests (build only)

Access images at: `ghcr.io/emanuelefumagalli/test-agent:latest`

## Example Output

```
15-day London Heathrow wind forecast (km/h):
Date        | Wind Max | Gust Max | Dir
------------+----------+---------+------
2026-01-30 |     20.2 |    41.4 | SE
2026-01-31 |     16.2 |    40.7 | S
2026-02-01 |      9.1 |    18.0 | SSE
2026-02-02 |     15.8 |    31.3 | E
2026-02-03 |     17.7 |    38.5 | E
2026-02-04 |     12.3 |    25.4 | SE
2026-02-05 |     13.7 |    27.0 | ENE
2026-02-06 |     14.3 |    32.0 | S
2026-02-07 |     11.2 |    32.8 | SW
2026-02-08 |     11.4 |    33.1 | SE
2026-02-09 |     24.9 |    44.6 | E
2026-02-10 |     25.0 |    42.5 | NE
2026-02-11 |     17.4 |    30.6 | N
2026-02-12 |     14.6 |    29.2 | WNW
2026-02-13 |     17.1 |    41.8 | SSE

Ollama summary:
The wind forecast for London Heathrow shows moderate conditions for the next 15 days...
```

## License

MIT
