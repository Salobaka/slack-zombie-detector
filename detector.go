package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	linearRe = regexp.MustCompile(`linear\.app/\S+`)
	githubPR = regexp.MustCompile(`github\.com/[^/]+/[^/]+/pull/\d+`)
)

type ActivityStatus struct {
	HasLinear bool
	HasGitHub bool
}

type MemberReport struct {
	UserID      string
	DisplayName string
	Missing     []string
}

type Report struct {
	Mode         string
	Date         string
	Zombies      []MemberReport
	ActiveCount  int
	TotalCount   int
	ChannelName  string
}

func DetectZombies(client *SlackClient, cfg *Config, mode string) (*Report, error) {
	now := time.Now()
	var oldest time.Time
	switch mode {
	case "weekly":
		oldest = now.Add(-7 * 24 * time.Hour)
	default:
		oldest = now.Add(-24 * time.Hour)
	}

	members, err := client.FetchMembers(cfg.ChannelID)
	if err != nil {
		return nil, err
	}

	// Resolve display names and filter whitelist
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

	// Build activity map
	activity := make(map[string]*ActivityStatus)
	for _, m := range tracked {
		activity[m.id] = &ActivityStatus{}
	}

	for _, msg := range messages {
		status, ok := activity[msg.User]
		if !ok {
			continue
		}
		if linearRe.MatchString(msg.Text) {
			status.HasLinear = true
		}
		if githubPR.MatchString(msg.Text) {
			status.HasGitHub = true
		}
	}

	// Build zombie list
	var zombies []MemberReport
	for _, m := range tracked {
		status := activity[m.id]
		var missing []string
		if !status.HasLinear {
			missing = append(missing, "Linear task")
		}
		if !status.HasGitHub {
			missing = append(missing, "GitHub PR")
		}
		if len(missing) > 0 {
			zombies = append(zombies, MemberReport{
				UserID:      m.id,
				DisplayName: m.name,
				Missing:     missing,
			})
		}
	}

	return &Report{
		Mode:        mode,
		Date:        now.Format("2006-01-02"),
		Zombies:     zombies,
		ActiveCount: len(tracked) - len(zombies),
		TotalCount:  len(tracked),
		ChannelName: cfg.ChannelName,
	}, nil
}

func FormatReport(r *Report) string {
	var b strings.Builder

	modeLabel := "Daily"
	if r.Mode == "weekly" {
		modeLabel = "Weekly"
	}

	fmt.Fprintf(&b, ":zombie: Zombie Report (%s — %s)\n\n", modeLabel, r.Date)

	if len(r.Zombies) == 0 {
		b.WriteString("Everyone posted activity! No zombies detected.\n")
	} else {
		b.WriteString("No activity detected from:\n")
		for _, z := range r.Zombies {
			fmt.Fprintf(&b, "• @%s — missing: %s\n", z.DisplayName, strings.Join(z.Missing, ", "))
		}
	}

	fmt.Fprintf(&b, "\nActive members: %d/%d\n", r.ActiveCount, r.TotalCount)
	fmt.Fprintf(&b, "Channel: #%s\n", r.ChannelName)

	return b.String()
}
