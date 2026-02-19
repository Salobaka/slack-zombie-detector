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

func DetectZombies(client *SlackClient, cfg *Config, mode string) (*Report, error) {
	now := time.Now()
	var oldest time.Time

	timeMode := mode
	if mode == "deep-scan" {
		timeMode = "weekly"
	}
	switch timeMode {
	case "weekly":
		oldest = now.AddDate(0, 0, -7)
	default:
		oldest = now.AddDate(0, 0, -1)
	}
	oldest = time.Date(oldest.Year(), oldest.Month(), oldest.Day(), 0, 0, 0, 0, oldest.Location())

	// Collect members from the first configured channel (primary)
	members, err := client.FetchMembers(cfg.Channels[0].ID)
	if err != nil {
		return nil, err
	}

	type member struct {
		id   string
		name string
	}
	var tracked []member
	for _, uid := range members {
		name, _ := client.GetUserDisplayName(uid)
		if cfg.IsWhitelisted(uid, name) {
			continue
		}
		tracked = append(tracked, member{id: uid, name: name})
	}

	// Determine which channels to scan
	type scanChannel struct {
		id   string
		name string
	}
	var toScan []scanChannel

	scanClient := client
	if mode == "deep-scan" {
		if cfg.UserToken == "" {
			return nil, fmt.Errorf("user_token is required for deep-scan mode")
		}
		scanClient = NewSlackClient(cfg.UserToken)
		userChannels, err := scanClient.FetchBotChannels()
		if err != nil {
			return nil, err
		}
		for _, ch := range userChannels {
			toScan = append(toScan, scanChannel{id: ch.ID, name: ch.Name})
		}
	} else {
		for _, ch := range cfg.Channels {
			toScan = append(toScan, scanChannel{id: ch.ID, name: ch.Name})
		}
	}

	// Scan channels for GitHub PR activity
	hasActivity := make(map[string]bool)
	var scannedNames []string
	for _, ch := range toScan {
		messages, err := scanClient.FetchMessages(ch.id, oldest, now)
		if err != nil {
			// Skip channels where access is denied
			continue
		}
		scannedNames = append(scannedNames, ch.name)
		for _, msg := range messages {
			if githubPR.MatchString(msg.Text) {
				hasActivity[msg.User] = true
			}
		}
	}

	var royalZombies, otherZombies []MemberReport
	for _, m := range tracked {
		if hasActivity[m.id] {
			continue
		}
		mr := MemberReport{UserID: m.id, DisplayName: m.name}
		if cfg.IsRoyal(m.id, m.name) {
			royalZombies = append(royalZombies, mr)
		} else {
			otherZombies = append(otherZombies, mr)
		}
	}

	channelNames := scannedNames
	if mode != "deep-scan" {
		channelNames = nil
		for _, ch := range toScan {
			channelNames = append(channelNames, ch.name)
		}
	}

	totalZombies := len(royalZombies) + len(otherZombies)
	return &Report{
		Mode:         mode,
		From:         oldest,
		To:           now,
		RoyalZombies: royalZombies,
		OtherZombies: otherZombies,
		ActiveCount:  len(tracked) - totalZombies,
		TotalCount:   len(tracked),
		Channels:     channelNames,
	}, nil
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

	fromStr := r.From.Format("2006-01-02 15:04")
	toStr := r.To.Format("2006-01-02 15:04")
	fmt.Fprintf(&b, ":zombie: Zombie Report (%s — %s to %s)\n\n", modeLabel, fromStr, toStr)

	if len(r.RoyalZombies) == 0 && len(r.OtherZombies) == 0 {
		b.WriteString("Everyone posted activity! No zombies detected.\n")
	} else {
		if len(r.RoyalZombies) > 0 {
			b.WriteString(":crown: *Royal Members*\n")
			for _, z := range r.RoyalZombies {
				fmt.Fprintf(&b, "• @%s\n", z.DisplayName)
			}
			b.WriteString("\n")
		}
		if len(r.OtherZombies) > 0 {
			b.WriteString(":busts_in_silhouette: *Other Members*\n")
			for _, z := range r.OtherZombies {
				fmt.Fprintf(&b, "• @%s\n", z.DisplayName)
			}
		}
	}

	fmt.Fprintf(&b, "\nActive members: %d/%d\n", r.ActiveCount, r.TotalCount)
	fmt.Fprintf(&b, "Channels scanned: ")
	for i, name := range r.Channels {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "#%s", name)
	}
	b.WriteString("\n")

	return b.String()
}
