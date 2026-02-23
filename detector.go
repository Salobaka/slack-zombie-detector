package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var githubPR = regexp.MustCompile(`github\.com/[^/]+/[^/]+/pull/\d+`)

type MessageLink struct {
	ChannelID string
	Timestamp string
}

func (m MessageLink) URL(workspace string) string {
	ts := strings.Replace(m.Timestamp, ".", "", 1)
	return fmt.Sprintf("https://%s.slack.com/archives/%s/p%s", workspace, m.ChannelID, ts)
}

type MemberReport struct {
	UserID      string
	DisplayName string
}

type ActiveMember struct {
	DisplayName string
	Messages    []MessageLink
}

type Report struct {
	Mode         string
	From         time.Time
	To           time.Time
	Workspace    string
	RoyalZombies []MemberReport
	OtherZombies []MemberReport
	Active       []ActiveMember
	TotalCount   int
	Channels     []string
}

type scanTarget struct {
	id   string
	name string
}

type member struct {
	id   string
	name string
}

func DetectZombies(client *SlackClient, cfg *Config, mode string) (*Report, error) {
	from, to := timeRange(mode)

	tracked, err := resolveMembers(client, cfg)
	if err != nil {
		return nil, err
	}

	scanClient, targets, err := scanTargets(client, cfg, mode)
	if err != nil {
		return nil, err
	}

	userMessages, scanned := scanForPRs(scanClient, targets, from, to)

	// Build tracked member set for quick lookup
	trackedSet := make(map[string]string) // id -> name
	for _, m := range tracked {
		trackedSet[m.id] = m.name
	}

	var royalZombies, otherZombies []MemberReport
	var active []ActiveMember

	for _, m := range tracked {
		msgs, found := userMessages[m.id]
		if found {
			active = append(active, ActiveMember{
				DisplayName: m.name,
				Messages:    msgs,
			})
		} else {
			mr := MemberReport{UserID: m.id, DisplayName: m.name}
			if cfg.IsRoyal(m.id, m.name) {
				royalZombies = append(royalZombies, mr)
			} else {
				otherZombies = append(otherZombies, mr)
			}
		}
	}

	channels := scanned
	if mode != "deep-scan" {
		channels = targetNames(targets)
	}

	return &Report{
		Mode:         mode,
		From:         from,
		To:           to,
		Workspace:    cfg.Workspace,
		RoyalZombies: royalZombies,
		OtherZombies: otherZombies,
		Active:       active,
		TotalCount:   len(tracked),
		Channels:     channels,
	}, nil
}

func timeRange(mode string) (from, to time.Time) {
	to = time.Now()
	days := -1
	switch mode {
	case "weekly":
		days = -7
	case "deep-scan":
		days = -2
	}
	from = to.AddDate(0, 0, days)
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	return
}

func resolveMembers(client *SlackClient, cfg *Config) ([]member, error) {
	ids, err := client.FetchMembers(cfg.Channels[0].ID)
	if err != nil {
		return nil, err
	}

	var tracked []member
	for _, uid := range ids {
		name, _ := client.GetUserDisplayName(uid)
		if cfg.IsWhitelisted(uid, name) {
			continue
		}
		tracked = append(tracked, member{id: uid, name: name})
	}
	return tracked, nil
}

func scanTargets(client *SlackClient, cfg *Config, mode string) (*SlackClient, []scanTarget, error) {
	if mode != "deep-scan" {
		targets := make([]scanTarget, len(cfg.Channels))
		for i, ch := range cfg.Channels {
			targets[i] = scanTarget{id: ch.ID, name: ch.Name}
		}
		return client, targets, nil
	}

	if cfg.UserToken == "" {
		return nil, nil, fmt.Errorf("user_token is required for deep-scan mode")
	}

	userClient := NewSlackClient(cfg.UserToken)
	channels, err := userClient.FetchAllChannels()
	if err != nil {
		return nil, nil, err
	}

	targets := make([]scanTarget, len(channels))
	for i, ch := range channels {
		targets[i] = scanTarget{id: ch.ID, name: ch.Name}
	}
	return userClient, targets, nil
}

func scanForPRs(client *SlackClient, targets []scanTarget, from, to time.Time) (map[string][]MessageLink, []string) {
	userMessages := make(map[string][]MessageLink)
	var scanned []string

	for _, ch := range targets {
		messages, err := client.FetchMessages(ch.id, from, to)
		if err != nil {
			continue
		}
		scanned = append(scanned, ch.name)
		for _, msg := range messages {
			if githubPR.MatchString(msg.Text) {
				link := MessageLink{ChannelID: ch.id, Timestamp: msg.Timestamp}
				userMessages[msg.User] = append(userMessages[msg.User], link)
			}
		}
	}

	return userMessages, scanned
}

func targetNames(targets []scanTarget) []string {
	names := make([]string, len(targets))
	for i, t := range targets {
		names[i] = t.name
	}
	return names
}

func FormatReport(r *Report) string {
	var b strings.Builder

	var modeLabel string
	switch r.Mode {
	case "weekly":
		modeLabel = "Weekly"
	case "deep-scan":
		modeLabel = "Deep Scan"
	default:
		modeLabel = "Daily"
	}

	fmt.Fprintf(&b, ":zombie: Zombie Report (%s — %s to %s)\n\n",
		modeLabel,
		r.From.Format("2006-01-02 15:04"),
		r.To.Format("2006-01-02 15:04"),
	)

	// Zombies
	totalZombies := len(r.RoyalZombies) + len(r.OtherZombies)
	if totalZombies == 0 {
		b.WriteString("Everyone posted activity! No zombies detected.\n\n")
	} else {
		writeZombieGroup(&b, ":crown: *Royal Members*", r.RoyalZombies)
		writeZombieGroup(&b, ":busts_in_silhouette: *Other Members*", r.OtherZombies)
	}

	// Active members
	if len(r.Active) > 0 {
		b.WriteString(":white_check_mark: *Active Members*\n")
		for _, a := range r.Active {
			var links []string
			for i, msg := range a.Messages {
				url := msg.URL(r.Workspace)
				links = append(links, fmt.Sprintf("<%s|%d>", url, i+1))
			}
			fmt.Fprintf(&b, "• @%s — %s\n", a.DisplayName, strings.Join(links, " "))
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Active: %d/%d | Channels: %d\n", len(r.Active), r.TotalCount, len(r.Channels))

	return b.String()
}

func writeZombieGroup(b *strings.Builder, header string, members []MemberReport) {
	if len(members) == 0 {
		return
	}
	fmt.Fprintf(b, "%s\n", header)
	for _, z := range members {
		fmt.Fprintf(b, "• @%s\n", z.DisplayName)
	}
	b.WriteString("\n")
}
