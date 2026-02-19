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
	configPath := flag.String("config", "config.yaml", "Path to config file")
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

	report, err := DetectZombies(client, cfg, *mode)
	if err != nil {
		log.Fatalf("detect: %v", err)
	}

	text := FormatReport(report)

	if *dryRun {
		fmt.Print(text)
		return
	}

	if err := client.SendDM(cfg.ReportRecipient, text); err != nil {
		log.Fatalf("send: %v", err)
	}

	fmt.Println("Report sent.")
}
