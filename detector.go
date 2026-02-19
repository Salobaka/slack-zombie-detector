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
	ChannelName  string
}

func DetectZombies(client *SlackClient, cfg *Config, mode string) (*Report, error) {
	now := time.Now()
	var oldest time.Time
	switch mode {
	case "weekly":
		oldest = now.AddDate(0, 0, -7)
	default:
		oldest = now.AddDate(0, 0, -1)
	}
	oldest = time.Date(oldest.Year(), oldest.Month(), oldest.Day(), 0, 0, 0, 0, oldest.Location())

	members, err := client.FetchMembers(cfg.ChannelID)
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

	messages, err := client.FetchMessages(cfg.ChannelID, oldest, now)
	if err != nil {
		return nil, err
	}

	hasActivity := make(map[string]bool)
	for _, msg := range messages {
		if githubPR.MatchString(msg.Text) {
			hasActivity[msg.User] = true
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

	totalZombies := len(royalZombies) + len(otherZombies)
	return &Report{
		Mode:         mode,
		From:         oldest,
		To:           now,
		RoyalZombies: royalZombies,
		OtherZombies: otherZombies,
		ActiveCount:  len(tracked) - totalZombies,
		TotalCount:   len(tracked),
		ChannelName:  cfg.ChannelName,
	}, nil
}

func FormatReport(r *Report) string {
	var b strings.Builder

	modeLabel := "Daily"
	if r.Mode == "weekly" {
		modeLabel = "Weekly"
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
	fmt.Fprintf(&b, "Channel: #%s\n", r.ChannelName)

	return b.String()
}
