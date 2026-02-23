package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/slack-go/slack"
)

type SlackClient struct {
	api *slack.Client
}

func NewSlackClient(token string) *SlackClient {
	return &SlackClient{api: slack.New(token)}
}

func (sc *SlackClient) FetchMessages(channelID string, oldest, latest time.Time) ([]slack.Message, error) {
	var all []slack.Message
	cursor := ""
	for {
		resp, err := sc.api.GetConversationHistory(&slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Oldest:    strconv.FormatInt(oldest.Unix(), 10),
			Latest:    strconv.FormatInt(latest.Unix(), 10),
			Limit:     200,
			Cursor:    cursor,
		})
		if err = sc.retryOrFail(err); err != nil {
			return nil, fmt.Errorf("fetching messages: %w", err)
		}
		if resp == nil {
			continue
		}
		all = append(all, resp.Messages...)
		if !resp.HasMore {
			return all, nil
		}
		cursor = resp.ResponseMetaData.NextCursor
	}
}

func (sc *SlackClient) FetchMembers(channelID string) ([]string, error) {
	var all []string
	cursor := ""
	for {
		members, nextCursor, err := sc.api.GetUsersInConversation(&slack.GetUsersInConversationParameters{
			ChannelID: channelID, Cursor: cursor, Limit: 200,
		})
		if err = sc.retryOrFail(err); err != nil {
			return nil, fmt.Errorf("fetching members: %w", err)
		}
		if members == nil {
			continue
		}
		all = append(all, members...)
		if nextCursor == "" {
			return all, nil
		}
		cursor = nextCursor
	}
}

func (sc *SlackClient) FetchAllChannels() ([]slack.Channel, error) {
	var all []slack.Channel
	cursor := ""
	for {
		channels, nextCursor, err := sc.api.GetConversations(&slack.GetConversationsParameters{
			Types: []string{"public_channel", "private_channel"}, ExcludeArchived: true,
			Limit: 200, Cursor: cursor,
		})
		if err = sc.retryOrFail(err); err != nil {
			return nil, fmt.Errorf("fetching channels: %w", err)
		}
		if channels == nil {
			continue
		}
		all = append(all, channels...)
		if nextCursor == "" {
			return all, nil
		}
		cursor = nextCursor
	}
}

// FetchUserNames returns a map of userID -> display name for all workspace users.
func (sc *SlackClient) FetchUserNames() (map[string]string, error) {
	users, err := sc.api.GetUsers()
	if err != nil {
		return nil, fmt.Errorf("fetching users: %w", err)
	}
	names := make(map[string]string, len(users))
	for _, u := range users {
		if u.Deleted {
			continue
		}
		name := u.Profile.DisplayName
		if name == "" {
			name = u.RealName
		}
		if name == "" {
			name = u.Name
		}
		names[u.ID] = name
	}
	return names, nil
}

func (sc *SlackClient) GetUserDisplayName(userID string) (string, error) {
	user, err := sc.api.GetUserInfo(userID)
	if err != nil {
		return userID, err
	}
	if user.Profile.DisplayName != "" {
		return user.Profile.DisplayName, nil
	}
	if user.RealName != "" {
		return user.RealName, nil
	}
	return user.Name, nil
}

func (sc *SlackClient) SendDM(userID, text string) error {
	_, _, err := sc.api.PostMessage(userID, slack.MsgOptionText(text, false))
	if err != nil {
		return fmt.Errorf("sending DM: %w", err)
	}
	return nil
}

// retryOrFail returns nil if the error is a rate limit (caller should retry),
// or returns the original error otherwise.
func (sc *SlackClient) retryOrFail(err error) error {
	if err == nil {
		return nil
	}
	if rle, ok := err.(*slack.RateLimitedError); ok {
		time.Sleep(rle.RetryAfter)
		return nil
	}
	return err
}
