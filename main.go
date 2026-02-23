package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

var validModes = map[string]bool{
	"daily":     true,
	"weekly":    true,
	"deep-scan": true,
}

func main() {
	mode := flag.String("mode", "daily", "Report mode: daily, weekly, or deep-scan")
	days := flag.Int("days", 0, "Override time range in days (0 = use mode default)")
	configPath := flag.String("config", "config.yaml", "Path to config file")
	byDay := flag.Bool("by-day", false, "Group active member activity by day")
	dryRun := flag.Bool("dry-run", false, "Print report to stdout instead of sending DM")
	flag.Parse()

	if !validModes[*mode] {
		fmt.Fprintf(os.Stderr, "invalid mode %q: must be daily, weekly, or deep-scan\n", *mode)
		os.Exit(1)
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	client := NewSlackClient(cfg.SlackToken)

	report, err := DetectZombies(client, cfg, *mode, *days, *byDay)
	if err != nil {
		log.Fatalf("detect: %v", err)
	}

	messages := FormatReport(report)

	if *dryRun {
		for _, msg := range messages {
			fmt.Print(msg)
		}
		return
	}

	for _, msg := range messages {
		if err := client.SendDM(cfg.ReportRecipient, msg); err != nil {
			log.Fatalf("send: %v", err)
		}
	}

	fmt.Printf("Report sent (%d messages).\n", len(messages))
}
