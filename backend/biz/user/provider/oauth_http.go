package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	oauthHTTPTimeout     = 15 * time.Second
	oauthHTTPBodyMaxSize = 1 << 20
	syntheticEmailDomain = "oauth.authyide.invalid"
)

// syntheticEmail 为不返回邮箱的 provider 派生只读邮箱，保证用户身份唯一且不占用真实邮箱段。
func syntheticEmail(name Name, identityID string) string {
	local := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, strings.ToLower(strings.TrimSpace(identityID)))
	return fmt.Sprintf("%s_%s@%s", string(name), local, syntheticEmailDomain)
}

func oauthClient(base *http.Client) *http.Client {
	if base != nil {
		return base
	}
	return &http.Client{Timeout: oauthHTTPTimeout}
}

func doGetJSON[Resp any](ctx context.Context, client *http.Client, rawURL string) (*Resp, error) {
	return doRequest[Resp](ctx, client, http.MethodGet, rawURL, nil, nil)
}

func doPostJSON[Resp any](ctx context.Context, client *http.Client, rawURL string, body any) (*Resp, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return doRequest[Resp](ctx, client, http.MethodPost, rawURL, bytes.NewReader(payload), http.Header{"Content-Type": []string{"application/json"}})
}

func doGetText(ctx context.Context, client *http.Client, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, oauthHTTPBodyMaxSize))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func doRequest[Resp any](ctx context.Context, client *http.Client, method, rawURL string, body io.Reader, header http.Header) (*Resp, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	for key, values := range header {
		req.Header[key] = values
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	var out Resp
	if err := json.NewDecoder(io.LimitReader(resp.Body, oauthHTTPBodyMaxSize)).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
