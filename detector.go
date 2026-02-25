package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var githubPR = regexp.MustCompile(`github\.com/[^/]+/[^/]+/pull/\d+`)

type MessageLink struct {
	ChannelID string
	Timestamp string
	PRURL     string
}

func (m MessageLink) URL(workspace string) string {
	return fmt.Sprintf("https://%s.slack.com/archives/%s/p%s",
		workspace, m.ChannelID, strings.Replace(m.Timestamp, ".", "", 1))
}

func (m MessageLink) Time() time.Time {
	parts := strings.Split(m.Timestamp, ".")
	sec, _ := strconv.ParseInt(parts[0], 10, 64)
	return time.Unix(sec, 0)
}

type MemberReport struct{ DisplayName string }
type ActiveMember struct {
	DisplayName string
	Messages    []MessageLink
}

type Report struct {
	Mode, Workspace  string
	From, To         time.Time
	ByDay            bool
	RoyalZombies     []MemberReport
	OtherZombies     []MemberReport
	Active           []ActiveMember
	TotalCount       int
	ChannelCount     int
}

type scanTarget struct{ id, name string }
type member struct{ id, name string }

func DetectZombies(client *SlackClient, cfg *Config, mode string, daysOverride int, byDay bool) (*Report, error) {
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
			name, _ = client.GetUserDisplayName(uid)
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
		From: from, To: to, ByDay: byDay,
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
		days = 1
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
			if pr := githubPR.FindString(msg.Text); pr != "" {
				userMsgs[msg.User] = append(userMsgs[msg.User], MessageLink{ch.id, msg.Timestamp, pr})
			}
		}
	}
	return userMsgs, scanned
}

const slackMaxLen = 3500

func FormatReport(r *Report) []string {
	modeLabels := map[string]string{"weekly": "Weekly", "deep-scan": "Deep Scan"}
	label := modeLabels[r.Mode]
	if label == "" {
		label = "Daily"
	}

	// Collect all blocks (each block = one logical unit that shouldn't be split)
	var blocks []string

	blocks = append(blocks, fmt.Sprintf(":zombie: Zombie Report (%s — %s to %s)\n",
		label, r.From.Format("Mon 2006-01-02 15:04"), r.To.Format("Mon 2006-01-02 15:04")))

	if len(r.RoyalZombies)+len(r.OtherZombies) == 0 {
		blocks = append(blocks, "Everyone posted activity! No zombies detected.\n")
	} else {
		if s := formatGroup(":crown: *Royal Members*", r.RoyalZombies); s != "" {
			blocks = append(blocks, s)
		}
		if s := formatGroup(":busts_in_silhouette: *Other Members*", r.OtherZombies); s != "" {
			blocks = append(blocks, s)
		}
	}

	if len(r.Active) > 0 {
		blocks = append(blocks, ":white_check_mark: *Active Members*\n")
		for _, a := range r.Active {
			blocks = append(blocks, formatActiveMember(a, r.ByDay, r.Workspace))
		}
	}

	blocks = append(blocks, fmt.Sprintf("\nActive: %d/%d | Channels: %d\n", len(r.Active), r.TotalCount, r.ChannelCount))

	// Pack blocks into messages without exceeding Slack limit
	var messages []string
	var current strings.Builder
	for _, block := range blocks {
		if current.Len()+len(block) > slackMaxLen && current.Len() > 0 {
			messages = append(messages, current.String())
			current.Reset()
		}
		current.WriteString(block)
	}
	if current.Len() > 0 {
		messages = append(messages, current.String())
	}
	return messages
}

func formatActiveMember(a ActiveMember, byDay bool, workspace string) string {
	var b strings.Builder
	if byDay {
		dayParts := formatDayLinks(a.Messages, workspace)
		fmt.Fprintf(&b, "@%s — %s\n", a.DisplayName, strings.Join(dayParts, " "))
	} else {
		type prEntry struct {
			link  MessageLink
			count int
		}
		seen := make(map[string]*prEntry)
		var order []string
		for _, msg := range a.Messages {
			if e, ok := seen[msg.PRURL]; ok {
				e.count++
			} else {
				seen[msg.PRURL] = &prEntry{link: msg, count: 1}
				order = append(order, msg.PRURL)
			}
		}
		links := make([]string, len(order))
		for i, pr := range order {
			e := seen[pr]
			if e.count > 1 {
				links[i] = fmt.Sprintf("<%s|%d>(%d)", e.link.URL(workspace), i+1, e.count)
			} else {
				links[i] = fmt.Sprintf("<%s|%d>", e.link.URL(workspace), i+1)
			}
		}
		fmt.Fprintf(&b, "@%s — %s\n", a.DisplayName, strings.Join(links, " "))
	}
	return b.String()
}

func formatGroup(header string, members []MemberReport) string {
	if len(members) == 0 {
		return ""
	}
	names := make([]string, len(members))
	for i, z := range members {
		names[i] = "@" + z.DisplayName
	}
	return fmt.Sprintf("%s\n%s\n\n", header, strings.Join(names, " | "))
}

func formatDayLinks(msgs []MessageLink, workspace string) []string {
	type dayGroup struct {
		date time.Time
		msgs []MessageLink
	}
	groups := make(map[string]*dayGroup)
	for _, msg := range msgs {
		t := msg.Time()
		key := t.Format("2006-01-02")
		if g, ok := groups[key]; ok {
			g.msgs = append(g.msgs, msg)
		} else {
			groups[key] = &dayGroup{date: t, msgs: []MessageLink{msg}}
		}
	}
	sorted := make([]*dayGroup, 0, len(groups))
	for _, g := range groups {
		sorted = append(sorted, g)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].date.Before(sorted[j].date) })

	type dayEntry struct {
		text    string
		newWeek bool
	}
	var entries []dayEntry
	for i, g := range sorted {
		newWeek := false
		if i > 0 {
			prevWeekday := sorted[i-1].date.Weekday()
			currWeekday := g.date.Weekday()
			if prevWeekday == time.Sunday || (currWeekday != time.Sunday && currWeekday < prevWeekday) {
				newWeek = true
			}
		}

		// Deduplicate by PR URL, keep first message link per unique PR
		type prEntry struct {
			link  MessageLink
			count int
		}
		seen := make(map[string]*prEntry)
		var prOrder []string
		for _, msg := range g.msgs {
			if e, ok := seen[msg.PRURL]; ok {
				e.count++
			} else {
				seen[msg.PRURL] = &prEntry{link: msg, count: 1}
				prOrder = append(prOrder, msg.PRURL)
			}
		}

		links := make([]string, len(prOrder))
		for j, pr := range prOrder {
			e := seen[pr]
			if e.count > 1 {
				links[j] = fmt.Sprintf("<%s|%d>(%d)", e.link.URL(workspace), j+1, e.count)
			} else {
				links[j] = fmt.Sprintf("<%s|%d>", e.link.URL(workspace), j+1)
			}
		}

		dayLabel := g.date.Format("Mon")
		if wd := g.date.Weekday(); wd == time.Saturday || wd == time.Sunday {
			dayLabel = fmt.Sprintf("*%s*", dayLabel)
		}

		entries = append(entries, dayEntry{
			text:    fmt.Sprintf("%s %d: %s", dayLabel, g.date.Day(), strings.Join(links, " ")),
			newWeek: newWeek,
		})
	}

	var parts []string
	for i, e := range entries {
		if i > 0 {
			if e.newWeek {
				parts = append(parts, "||")
			} else {
				parts = append(parts, "|")
			}
		}
		parts = append(parts, e.text)
	}
	return parts
}

