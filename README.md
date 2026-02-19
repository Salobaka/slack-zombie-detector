# Slack Zombie Detector

Monitors a Slack channel and reports which members didn't post a GitHub PR link during a given period. Sends a DM report to a configured recipient.

## First-Time Setup

### 1. Create a Slack App

1. Go to https://api.slack.com/apps and click **Create New App** > **From scratch**
2. Name it and select your workspace
3. Go to **OAuth & Permissions** > **Bot Token Scopes** and add:
   - `channels:history`, `channels:read` — read messages and list members
   - `groups:history`, `groups:read` — same for private channels
   - `users:read` — resolve display names
   - `chat:write`, `im:write` — send DM reports
4. Click **Install to Workspace** and approve
5. Copy the **Bot User OAuth Token** (`xoxb-...`)

### 2. Add the Bot to Your Channel

In Slack, open the channel and type `/invite @YourAppName`.

### 3. Find Your User ID

Click your profile picture > **Profile** > three dots **(...)** > **Copy member ID**.

### 4. Create Your Config

```bash
cp config.yaml.example config.yaml
```

Edit `config.yaml` with your token, user ID, and whitelist.

### 5. Build and Run

```bash
go build .
./slack-zombie-detector --mode=daily --dry-run   # preview
./slack-zombie-detector --mode=daily              # send DM
```

## Daily Use with Cron

```cron
# Daily at 9:00 AM Mon-Fri
0 9 * * 1-5 /full/path/to/slack-zombie-detector --mode=daily --config=/full/path/to/config.yaml

# Weekly on Monday at 9:00 AM
0 9 * * 1 /full/path/to/slack-zombie-detector --mode=weekly --config=/full/path/to/config.yaml
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--mode` | `daily` | `daily` (last 24h) or `weekly` (last 7 days) |
| `--config` | `config.yaml` | Path to config file |
| `--dry-run` | `false` | Print report to stdout, don't send DM |

## Config

| Field | Description |
|-------|-------------|
| `slack_token` | Bot token (`xoxb-...`) |
| `channel_id` | Channel to monitor |
| `channel_name` | Channel name (used in report) |
| `report_recipient` | Your Slack user ID (receives DM) |
| `whitelist` | User IDs or display names to exclude |
| `royal_members` | User IDs or display names shown in a separate group |
