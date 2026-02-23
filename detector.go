package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var githubPR = regexp.MustCompile(`github\.com/[^/]+/[^/]+/pull/\d+`)

type MemberReport struct {
	UserID      string
	DisplayName string
}

type Report struct {
	Mode         string
	From         time.Time
	To           time.Time
	RoyalZombies []MemberReport
	OtherZombies []MemberReport
	ActiveCount  int
	TotalCount   int
	Channels     []string
}

type scanTarget struct {
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

	hasActivity, scanned := scanForPRs(scanClient, targets, from, to)

	royalZombies, otherZombies := classifyZombies(tracked, hasActivity, cfg)

	channels := scanned
	if mode != "deep-scan" {
		channels = targetNames(targets)
	}

	totalZombies := len(royalZombies) + len(otherZombies)
	return &Report{
		Mode:         mode,
		From:         from,
		To:           to,
		RoyalZombies: royalZombies,
		OtherZombies: otherZombies,
		ActiveCount:  len(tracked) - totalZombies,
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

type member struct {
	id   string
	name string
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

func scanForPRs(client *SlackClient, targets []scanTarget, from, to time.Time) (map[string]bool, []string) {
	hasActivity := make(map[string]bool)
	var scanned []string

	for _, ch := range targets {
		messages, err := client.FetchMessages(ch.id, from, to)
		if err != nil {
			continue
		}
		scanned = append(scanned, ch.name)
		for _, msg := range messages {
			if githubPR.MatchString(msg.Text) {
				hasActivity[msg.User] = true
			}
		}
	}

	return hasActivity, scanned
}

func classifyZombies(tracked []member, hasActivity map[string]bool, cfg *Config) (royal, other []MemberReport) {
	for _, m := range tracked {
		if hasActivity[m.id] {
			continue
		}
		mr := MemberReport{UserID: m.id, DisplayName: m.name}
		if cfg.IsRoyal(m.id, m.name) {
			royal = append(royal, mr)
		} else {
			other = append(other, mr)
		}
	}
	return
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

	if len(r.RoyalZombies) == 0 && len(r.OtherZombies) == 0 {
		b.WriteString("Everyone posted activity! No zombies detected.\n")
	} else {
		writeGroup(&b, ":crown: *Royal Members*", r.RoyalZombies)
		writeGroup(&b, ":busts_in_silhouette: *Other Members*", r.OtherZombies)
	}

	fmt.Fprintf(&b, "\nActive members: %d/%d\n", r.ActiveCount, r.TotalCount)
	fmt.Fprintf(&b, "Channels scanned: %d\n", len(r.Channels))

	return b.String()
}

func writeGroup(b *strings.Builder, header string, members []MemberReport) {
	if len(members) == 0 {
		return
	}
	fmt.Fprintf(b, "%s\n", header)
	for _, z := range members {
		fmt.Fprintf(b, "• @%s\n", z.DisplayName)
	}
	b.WriteString("\n")
}
