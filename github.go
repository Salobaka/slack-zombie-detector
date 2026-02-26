package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type GitHubClient struct {
	token string
	org   string
}

func NewGitHubClient(token, org string) *GitHubClient {
	return &GitHubClient{token: token, org: org}
}

type GitHubPR struct {
	Author  string
	Title   string
	HTMLURL string
	Created time.Time
}

func (gc *GitHubClient) FetchPRs(from, to time.Time) ([]GitHubPR, error) {
	var all []GitHubPR
	page := 1

	for {
		query := fmt.Sprintf("org:%s is:pr created:%s..%s",
			gc.org, from.Format("2006-01-02"), to.Format("2006-01-02"))

		u := fmt.Sprintf("https://api.github.com/search/issues?q=%s&per_page=100&page=%d",
			url.QueryEscape(query), page)

		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+gc.token)
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("github api: %w", err)
		}

		var result struct {
			Items []struct {
				User struct {
					Login string `json:"login"`
				} `json:"user"`
				Title   string `json:"title"`
				HTMLURL string `json:"html_url"`
				Created string `json:"created_at"`
			} `json:"items"`
			TotalCount int `json:"total_count"`
		}

		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		for _, item := range result.Items {
			created, _ := time.Parse(time.RFC3339, item.Created)
			all = append(all, GitHubPR{
				Author:  item.User.Login,
				Title:   item.Title,
				HTMLURL: item.HTMLURL,
				Created: created,
			})
		}

		if len(all) >= result.TotalCount || len(result.Items) == 0 {
			break
		}
		page++
	}

	return all, nil
}
