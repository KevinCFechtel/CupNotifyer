# CupNotifyer

CupNotifyer polls [Cup](https://github.com/sergi0g/cup)'s v3 JSON API and sends a
Telegram message for every newly detected container image update.

The last successfully notified version or digest is stored on disk. This avoids
duplicate messages across polls and container restarts. Notification providers
are behind a small Go interface so additional targets can be added independently.

## Requirements

- A running Cup server with `/api/v3/json` available
- A Telegram bot token
- The target chat ID
- Docker Compose, or Go 1.24+

## Run with Docker Compose

1. Create the configuration:

   ```sh
   cp .env.example .env
   ```

2. Set `CUP_API_URL`, `TELEGRAM_BOT_TOKEN`, and `TELEGRAM_CHAT_ID` in `.env`.

3. Start the service:

   ```sh
   docker compose up -d --build
   ```

4. Follow its logs:

   ```sh
   docker compose logs -f cupnotifier
   ```

If Cup runs in a different Compose project, both containers need a shared Docker
network, or `CUP_API_URL` must point to an address reachable from this container.

On the first successful poll, all currently available updates are notified once.

Cup's JSON endpoint returns its latest cached check result. Configure Cup's
[`refresh_interval`](https://cup.sergi0g.dev/docs/configuration/automatic-refresh)
so Cup checks registries regularly; CupNotifyer then polls those results at
`POLL_INTERVAL`.

## Configuration

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `CUP_API_URL` | yes | - | Full Cup endpoint, e.g. `http://cup:8000/api/v3/json` |
| `TELEGRAM_BOT_TOKEN` | yes | - | Token created by BotFather |
| `TELEGRAM_CHAT_ID` | yes | - | User, group, or channel chat ID |
| `POLL_INTERVAL` | no | `5m` | Interval in Go duration syntax |
| `HTTP_TIMEOUT` | no | `15s` | Timeout for Cup and Telegram requests |
| `STATE_FILE` | no | `/data/state.json` | Persistent notification state |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error` |

The Compose setup stores `/data/state.json` in the named
`cupnotifier-data` volume.

## Run locally

```sh
export CUP_API_URL=http://localhost:8000/api/v3/json
export TELEGRAM_BOT_TOKEN=...
export TELEGRAM_CHAT_ID=...
export STATE_FILE=./data/state.json
go run ./cmd/cupnotifier
```

## Add another notification target

Implement `internal/notification.Notifier` and register the implementation in
`cmd/cupnotifier/main.go`. State is tracked separately for every notifier ID, so
a newly added target receives updates independently from Telegram.
