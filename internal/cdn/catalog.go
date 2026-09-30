package cdn

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Category is a node in the mediator tree. Child listings are not expanded.
type Category struct {
	Parent        string      `json:"parent,omitempty"`
	Children      []string    `json:"children,omitempty"`
	Key           string      `json:"key"`
	Name          string      `json:"name"`
	Subcategories []Category  `json:"subcategories"`
	Media         []MediaItem `json:"media"`
}

// Categories reads the root, or a detailed category by key.
func (c *Client) Categories(ctx context.Context, key string) (*Category, error) {
	base := c.CategoriesURL
	if base == "" {
		base = "https://b.jw-cdn.org/apis/mediator/v1/categories"
	}
	u := strings.TrimRight(base, "/") + "/" + url.PathEscape(c.Lang)
	if key != "" {
		u += "/" + url.PathEscape(key) + "?detailed=1"
	}
	var answer struct {
		Category   Category   `json:"category"`
		Categories []Category `json:"categories"`
	}
	if err := c.getJSON(ctx, u, &answer); err != nil {
		return nil, err
	}
	if len(answer.Categories) > 0 {
		answer.Category.Subcategories = answer.Categories
	}
	return &answer.Category, nil
}

// Pause waits between requests and remains cancellable.
func Pause(ctx context.Context, interval time.Duration) error {
	if interval < 0 {
		return fmt.Errorf("interval must not be negative")
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// GetLimited reads at most budget bytes of a subtitle and verifies its checksum.
// The extra byte detects a lying or missing Content-Length, without buffering a video.
func (c *Client) GetLimited(ctx context.Context, u string, budget int64, checksum string) ([]byte, int64, error) {
	if budget <= 0 {
		return nil, 0, fmt.Errorf("byte budget exhausted")
	}
	resp, err := c.get(ctx, u)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.ContentLength > budget {
		return nil, 0, fmt.Errorf("resource needs %d bytes, budget has %d", resp.ContentLength, budget)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, budget))
	n := int64(len(b))
	if err != nil {
		return nil, n, err
	}
	if n == budget && (resp.ContentLength < 0 || resp.ContentLength > n) {
		return nil, n, fmt.Errorf("resource reached byte budget")
	}
	sum := md5.Sum(b)
	if checksum != "" && !strings.EqualFold(hex.EncodeToString(sum[:]), checksum) {
		return nil, n, fmt.Errorf("subtitle checksum mismatch")
	}
	return b, n, nil
}

var mediaKeyPattern = regexp.MustCompile(`^(?:pub-|docid-)[A-Za-z0-9_-]+_(?:VIDEO|AUDIO)$`)

// ValidMediaKey also prevents catalog keys from becoming paths when VTT files are cached.
func ValidMediaKey(key string) bool { return mediaKeyPattern.MatchString(key) }
