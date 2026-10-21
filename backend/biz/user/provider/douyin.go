package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/chaitin/MonkeyCode/backend/config"
)

const (
	douyinAuthorizeURL = "https://open.douyin.com/platform/oauth/connect/authorize/"
	douyinTokenURL     = "https://open.douyin.com/oauth/access_token/"
	douyinUserInfoURL  = "https://open.douyin.com/oauth/userinfo.get/"
)

// DouyinProvider 对接抖音开放平台 OAuth2 登录，返回真实邮箱并校验 email_verified。
type DouyinProvider struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	redirectURL  string
}

func NewDouyin(cfg config.OAuthLoginProviderConfig, serverBaseURL string, httpClient *http.Client) *DouyinProvider {
	return &DouyinProvider{
		httpClient:   oauthClient(httpClient),
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		redirectURL:  oauthCallbackURL(cfg.RedirectURL, serverBaseURL, Douyin),
	}
}

func (p *DouyinProvider) Name() Name {
	return Douyin
}

func (p *DouyinProvider) AuthCodeURL(state, _ string) string {
	q := url.Values{}
	q.Set("client_key", p.clientID)
	q.Set("response_type", "code")
	q.Set("scope", "user_info")
	q.Set("state", state)
	return douyinAuthorizeURL + "?" + q.Encode()
}

type douyinErrorResponse struct {
	Code    string `json:"code"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e *douyinErrorResponse) failed() error {
	if e == nil || strings.TrimSpace(e.Code) == "ok" {
		return nil
	}
	return fmt.Errorf("douyin api error: code=%s type=%s message=%s", e.Code, e.Type, e.Message)
}

type douyinTokenData struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	OpenID           string `json:"open_id"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	Scope            string `json:"scope"`
}

type douyinTokenResponse struct {
	Data  douyinTokenData      `json:"data"`
	Error *douyinErrorResponse `json:"error"`
}

func (p *DouyinProvider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	body := map[string]string{
		"client_key":    p.clientID,
		"client_secret": p.clientSecret,
		"code":          code,
		"grant_type":    "authorization_code",
	}
	resp, err := doPostJSON[douyinTokenResponse](ctx, p.httpClient, douyinTokenURL, body)
	if err != nil {
		return nil, err
	}
	if err := resp.Error.failed(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(resp.Data.AccessToken) == "" {
		return nil, errors.New("douyin access token is required")
	}
	token := &oauth2.Token{
		AccessToken:  resp.Data.AccessToken,
		TokenType:    "bearer",
		RefreshToken: resp.Data.RefreshToken,
		ExpiresIn:    int64(resp.Data.ExpiresIn),
		Expiry:       time.Now().Add(time.Duration(resp.Data.ExpiresIn) * time.Second),
	}
	if openID := strings.TrimSpace(resp.Data.OpenID); openID != "" {
		token = token.WithExtra(map[string]any{"open_id": openID})
	}
	return token, nil
}

type douyinUserinfoData struct {
	OpenID        string `json:"open_id"`
	UnionID       string `json:"union_id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Avatar        string `json:"avatar"`
}

type douyinUserinfoResponse struct {
	Data  douyinUserinfoData   `json:"data"`
	Error *douyinErrorResponse `json:"error"`
}

func (p *DouyinProvider) User(ctx context.Context, token *oauth2.Token, _ string) (*ExternalUser, error) {
	if token == nil || strings.TrimSpace(token.AccessToken) == "" {
		return nil, errors.New("douyin access token is required")
	}
	resp, err := doPostJSON[douyinUserinfoResponse](ctx, p.httpClient, douyinUserInfoURL, map[string]string{"access_token": token.AccessToken})
	if err != nil {
		return nil, err
	}
	if err := resp.Error.failed(); err != nil {
		return nil, err
	}
	openID := strings.TrimSpace(resp.Data.OpenID)
	if openID == "" {
		openID = oauthTokenString(token, "open_id")
	}
	if openID == "" {
		return nil, errors.New("douyin open_id is required")
	}
	displayName := strings.TrimSpace(resp.Data.UnionID)
	if displayName == "" {
		displayName = openID
	}
	return &ExternalUser{
		Provider:      Douyin,
		IdentityID:    openID,
		Email:         strings.TrimSpace(resp.Data.Email),
		EmailVerified: resp.Data.EmailVerified,
		Username:      displayName,
		Name:          displayName,
		AvatarURL:     strings.TrimSpace(resp.Data.Avatar),
	}, nil
}
