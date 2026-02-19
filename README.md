# Slack Zombie Detector

Monitors a Slack channel and reports which dev members didn't post work activity (Linear task URL + GitHub PR URL) during a given period.

## First-Time Setup

### 1. Create a Slack App

1. Go to https://api.slack.com/apps and click **Create New App** > **From scratch**
2. Name it (e.g. "Zombie Detector") and select your workspace
3. Go to **OAuth & Permissions** > **Scopes** > **Bot Token Scopes** and add:
   - `channels:history` — read channel messages
   - `channels:read` — list channel members
   - `groups:history` — read private channel messages (if needed)
   - `groups:read` — list private channel members (if needed)
   - `users:read` — resolve user display names
   - `chat:write` — send DM reports
   - `im:write` — open DM conversations
4. Click **Install to Workspace** and approve
5. Copy the **Bot User OAuth Token** (`xoxb-...`)

### 2. Add the Bot to Your Channel

In Slack, go to **#medidrive-pr-review** > channel settings > **Integrations** > **Add apps** > select your bot.

### 3. Find Your User ID

In Slack, click your profile picture > **Profile** > click the **three dots (...)** > **Copy member ID**.

### 4. Create Your Config

```bash
cp config.yaml.example config.yaml
```

Edit `config.yaml`:

```yaml
slack_token: "xoxb-your-actual-token"
channel_id: "C0ABK4V1KGT"
channel_name: "medidrive-pr-review"
report_recipient: "U_YOUR_USER_ID"
whitelist:
  - "U_BOT_USER_ID"
  - "product.manager"
```

- **slack_token** — the bot token from step 1
- **report_recipient** — your user ID from step 3
- **whitelist** — user IDs or display names to exclude (bots, PMs, etc.)

### 5. Build

```bash
go build .
```

### 6. Test with Dry Run

```bash
./slack-zombie-detector --mode=daily --dry-run
```

This prints the report to your terminal without sending any DM. Verify it looks correct.

### 7. Send Your First Report

```bash
./slack-zombie-detector --mode=daily
```

You'll receive a DM from the bot with the zombie report.

## Daily Use with Cron

Open your crontab:

```bash
crontab -e
```

Add one of these lines:

```cron
# Daily at 9:00 AM Mon-Fri
0 9 * * 1-5 /full/path/to/slack-zombie-detector --mode=daily --config=/full/path/to/config.yaml

# Weekly summary on Monday at 9:00 AM
0 9 * * 1 /full/path/to/slack-zombie-detector --mode=weekly --config=/full/path/to/config.yaml
```

Replace `/full/path/to/` with actual paths. Find them with:

```bash
echo "$(pwd)/slack-zombie-detector"
echo "$(pwd)/config.yaml"
```

Verify cron is saved:

```bash
crontab -l
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--mode` | `daily` | `daily` (last 24h) or `weekly` (last 7 days) |
| `--config` | `config.yaml` | Path to config file |
| `--dry-run` | `false` | Print report to stdout, don't send DM |
