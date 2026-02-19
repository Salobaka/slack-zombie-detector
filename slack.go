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
		if err != nil {
			if rle, ok := err.(*slack.RateLimitedError); ok {
				time.Sleep(rle.RetryAfter)
				continue
			}
			return nil, fmt.Errorf("fetching messages: %w", err)
		}

		all = append(all, resp.Messages...)
		if !resp.HasMore {
			break
		}
		cursor = resp.ResponseMetaData.NextCursor
	}

	return all, nil
}

func (sc *SlackClient) FetchMembers(channelID string) ([]string, error) {
	var all []string
	cursor := ""

	for {
		members, nextCursor, err := sc.api.GetUsersInConversation(&slack.GetUsersInConversationParameters{
			ChannelID: channelID,
			Cursor:    cursor,
			Limit:     200,
		})
		if err != nil {
			if rle, ok := err.(*slack.RateLimitedError); ok {
				time.Sleep(rle.RetryAfter)
				continue
			}
			return nil, fmt.Errorf("fetching members: %w", err)
		}

		all = append(all, members...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return all, nil
}

func (sc *SlackClient) FetchAllChannels() ([]slack.Channel, error) {
	var all []slack.Channel
	cursor := ""

	for {
		channels, nextCursor, err := sc.api.GetConversations(&slack.GetConversationsParameters{
			Types:           []string{"public_channel", "private_channel"},
			ExcludeArchived: true,
			Limit:           200,
			Cursor:          cursor,
		})
		if err != nil {
			if rle, ok := err.(*slack.RateLimitedError); ok {
				time.Sleep(rle.RetryAfter)
				continue
			}
			return nil, fmt.Errorf("fetching channels: %w", err)
		}

		all = append(all, channels...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return all, nil
}

func (sc *SlackClient) GetUserDisplayName(userID string) (string, error) {
	user, err := sc.api.GetUserInfo(userID)
	if err != nil {
		return userID, fmt.Errorf("fetching user info: %w", err)
	}
	if user.Profile.DisplayName != "" {
		return user.Profile.DisplayName, nil
	}
	return user.RealName, nil
}

func (sc *SlackClient) SendDM(userID, text string) error {
	_, _, err := sc.api.PostMessage(userID, slack.MsgOptionText(text, false))
	if err != nil {
		return fmt.Errorf("sending DM: %w", err)
	}
	return nil
}
