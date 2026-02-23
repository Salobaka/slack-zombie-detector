package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var githubPR = regexp.MustCompile(`github\.com/[^/]+/[^/]+/pull/\d+`)

type MessageLink struct {
	ChannelID string
	Timestamp string
}

func (m MessageLink) URL(workspace string) string {
	return fmt.Sprintf("https://%s.slack.com/archives/%s/p%s",
		workspace, m.ChannelID, strings.Replace(m.Timestamp, ".", "", 1))
}

type MemberReport struct{ DisplayName string }
type ActiveMember struct {
	DisplayName string
	Messages    []MessageLink
}

type Report struct {
	Mode, Workspace  string
	From, To         time.Time
	RoyalZombies     []MemberReport
	OtherZombies     []MemberReport
	Active           []ActiveMember
	TotalCount       int
	ChannelCount     int
}

type scanTarget struct{ id, name string }
type member struct{ id, name string }

func DetectZombies(client *SlackClient, cfg *Config, mode string, daysOverride int) (*Report, error) {
	from, to := timeRange(mode, daysOverride)

	// Batch-fetch all user names (1 API call instead of N)
	names, err := client.FetchUserNames()
	if err != nil {
		return nil, err
	}

	memberIDs, err := client.FetchMembers(cfg.Channels[0].ID)
	if err != nil {
		return nil, err
	}

	var tracked []member
	for _, uid := range memberIDs {
		name := names[uid]
		if name == "" {
			name = uid
		}
		if cfg.IsWhitelisted(uid, name) {
			continue
		}
		tracked = append(tracked, member{uid, name})
	}

	scanClient, targets, err := scanTargets(client, cfg, mode)
	if err != nil {
		return nil, err
	}

	userMessages, channelCount := scanForPRs(scanClient, targets, from, to)

	var royalZombies, otherZombies []MemberReport
	var active []ActiveMember
	for _, m := range tracked {
		if msgs, ok := userMessages[m.id]; ok {
			active = append(active, ActiveMember{m.name, msgs})
		} else if cfg.IsRoyal(m.id, m.name) {
			royalZombies = append(royalZombies, MemberReport{m.name})
		} else {
			otherZombies = append(otherZombies, MemberReport{m.name})
		}
	}

	sortByName := func(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) }
	sort.Slice(royalZombies, func(i, j int) bool { return sortByName(royalZombies[i].DisplayName, royalZombies[j].DisplayName) })
	sort.Slice(otherZombies, func(i, j int) bool { return sortByName(otherZombies[i].DisplayName, otherZombies[j].DisplayName) })
	sort.Slice(active, func(i, j int) bool { return sortByName(active[i].DisplayName, active[j].DisplayName) })

	return &Report{
		Mode: mode, Workspace: cfg.Workspace,
		From: from, To: to,
		RoyalZombies: royalZombies, OtherZombies: otherZombies,
		Active: active, TotalCount: len(tracked), ChannelCount: channelCount,
	}, nil
}

func timeRange(mode string, daysOverride int) (from, to time.Time) {
	to = time.Now()
	days := 1
	switch mode {
	case "weekly":
		days = 7
	case "deep-scan":
		days = 2
	}
	if daysOverride > 0 {
		days = daysOverride
	}
	from = to.AddDate(0, 0, -days)
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	return
}

func scanTargets(client *SlackClient, cfg *Config, mode string) (*SlackClient, []scanTarget, error) {
	if mode != "deep-scan" {
		targets := make([]scanTarget, len(cfg.Channels))
		for i, ch := range cfg.Channels {
			targets[i] = scanTarget{ch.ID, ch.Name}
		}
		return client, targets, nil
	}
	if cfg.UserToken == "" {
		return nil, nil, fmt.Errorf("user_token is required for deep-scan mode")
	}
	uc := NewSlackClient(cfg.UserToken)
	channels, err := uc.FetchAllChannels()
	if err != nil {
		return nil, nil, err
	}
	targets := make([]scanTarget, len(channels))
	for i, ch := range channels {
		targets[i] = scanTarget{ch.ID, ch.Name}
	}
	return uc, targets, nil
}

func scanForPRs(client *SlackClient, targets []scanTarget, from, to time.Time) (map[string][]MessageLink, int) {
	userMsgs := make(map[string][]MessageLink)
	scanned := 0
	for _, ch := range targets {
		messages, err := client.FetchMessages(ch.id, from, to)
		if err != nil {
			continue
		}
		scanned++
		for _, msg := range messages {
			if githubPR.MatchString(msg.Text) {
				userMsgs[msg.User] = append(userMsgs[msg.User], MessageLink{ch.id, msg.Timestamp})
			}
		}
	}
	return userMsgs, scanned
}

func FormatReport(r *Report) string {
	var b strings.Builder

	modeLabels := map[string]string{"weekly": "Weekly", "deep-scan": "Deep Scan"}
	label := modeLabels[r.Mode]
	if label == "" {
		label = "Daily"
	}

	fmt.Fprintf(&b, ":zombie: Zombie Report (%s — %s to %s)\n\n",
		label, r.From.Format("Mon 2006-01-02 15:04"), r.To.Format("Mon 2006-01-02 15:04"))

	if len(r.RoyalZombies)+len(r.OtherZombies) == 0 {
		b.WriteString("Everyone posted activity! No zombies detected.\n\n")
	} else {
		writeGroup(&b, ":crown: *Royal Members*", r.RoyalZombies)
		writeGroup(&b, ":busts_in_silhouette: *Other Members*", r.OtherZombies)
	}

	if len(r.Active) > 0 {
		b.WriteString(":white_check_mark: *Active Members*\n")
		for _, a := range r.Active {
			links := make([]string, len(a.Messages))
			for i, msg := range a.Messages {
				links[i] = fmt.Sprintf("<%s|%d>", msg.URL(r.Workspace), i+1)
			}
			fmt.Fprintf(&b, "• @%s — %s\n", a.DisplayName, strings.Join(links, " "))
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Active: %d/%d | Channels: %d\n", len(r.Active), r.TotalCount, r.ChannelCount)
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
