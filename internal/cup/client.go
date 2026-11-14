package cup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/KevinCFechtel/CupNotifyer/internal/update"
)

const maxResponseSize = 10 << 20

type Client struct {
	apiURL     string
	httpClient *http.Client
}

type response struct {
	Images []image `json:"images"`
}

type image struct {
	Reference string  `json:"reference"`
	URL       *string `json:"url"`
	Result    result  `json:"result"`
	Server    *string `json:"server"`
}

type result struct {
	HasUpdate *bool       `json:"has_update"`
	Info      *updateInfo `json:"info"`
}

type updateInfo struct {
	Type              string   `json:"type"`
	VersionUpdateType string   `json:"version_update_type"`
	NewTag            string   `json:"new_tag"`
	CurrentVersion    string   `json:"current_version"`
	NewVersion        string   `json:"new_version"`
	LocalDigests      []string `json:"local_digests"`
	RemoteDigest      *string  `json:"remote_digest"`
}

func NewClient(apiURL string, httpClient *http.Client) *Client {
	return &Client{
		apiURL:     apiURL,
		httpClient: httpClient,
	}
}

func (c *Client) FetchUpdates(ctx context.Context) ([]update.Available, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create Cup request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CupNotifyer/1")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Cup API: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return nil, fmt.Errorf("Cup API returned %s: %s", res.Status, strings.TrimSpace(string(body)))
	}

	var payload response
	decoder := json.NewDecoder(io.LimitReader(res.Body, maxResponseSize))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Cup response: %w", err)
	}

	updates := make([]update.Available, 0, len(payload.Images))
	for _, image := range payload.Images {
		if image.Result.HasUpdate == nil || !*image.Result.HasUpdate {
			continue
		}
		if image.Result.Info == nil {
			return nil, fmt.Errorf("Cup reported an update for %q without update details", image.Reference)
		}

		item := update.Available{Reference: image.Reference}
		if image.Server != nil {
			item.Server = *image.Server
		}
		if image.URL != nil {
			item.URL = *image.URL
		}

		switch image.Result.Info.Type {
		case string(update.KindVersion):
			if image.Result.Info.NewTag == "" {
				return nil, fmt.Errorf("Cup reported a version update for %q without a new tag", image.Reference)
			}
			item.Kind = update.KindVersion
			item.UpdateType = image.Result.Info.VersionUpdateType
			item.CurrentVersion = image.Result.Info.CurrentVersion
			item.NewVersion = image.Result.Info.NewVersion
			item.NewTag = image.Result.Info.NewTag
		case string(update.KindDigest):
			if image.Result.Info.RemoteDigest == nil || *image.Result.Info.RemoteDigest == "" {
				return nil, fmt.Errorf("Cup reported a digest update for %q without a remote digest", image.Reference)
			}
			item.Kind = update.KindDigest
			item.LocalDigests = image.Result.Info.LocalDigests
			item.RemoteDigest = *image.Result.Info.RemoteDigest
		default:
			return nil, fmt.Errorf("Cup reported unsupported update type %q for %q", image.Result.Info.Type, image.Reference)
		}
		updates = append(updates, item)
	}

	return updates, nil
}
