package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/chaitin/MonkeyCode/backend/config"
)

const (
	qqAuthorizeURL = "https://graph.qq.com/oauth2.0/authorize"
	qqTokenURL     = "https://graph.qq.com/oauth2.0/token"
	qqMeURL        = "https://graph.qq.com/oauth2.0/me"
	qqUserInfoURL  = "https://graph.qq.com/user/get_user_info"
)

// QQProvider 对接 QQ 互联网页登录。QQ 不返回邮箱，用 provider 身份派生只读邮箱兜底。
type QQProvider struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	redirectURL  string
}

func NewQQ(cfg config.OAuthLoginProviderConfig, serverBaseURL string, httpClient *http.Client) *QQProvider {
	return &QQProvider{
		httpClient:   oauthClient(httpClient),
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		redirectURL:  oauthCallbackURL(cfg.RedirectURL, serverBaseURL, QQ),
	}
}

func (p *QQProvider) Name() Name {
	return QQ
}

func (p *QQProvider) AuthCodeURL(state, _ string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", p.clientID)
	q.Set("redirect_uri", p.redirectURL)
	q.Set("scope", "get_user_info")
	q.Set("state", state)
	return qqAuthorizeURL + "?" + q.Encode()
}

func (p *QQProvider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	q := url.Values{}
	q.Set("grant_type", "authorization_code")
	q.Set("client_id", p.clientID)
	q.Set("client_secret", p.clientSecret)
	q.Set("code", code)
	q.Set("redirect_uri", p.redirectURL)
	text, err := doGetText(ctx, p.httpClient, qqTokenURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	fields, err := url.ParseQuery(text)
	if err != nil {
		return nil, err
	}
	if message := fields.Get("error"); message != "" {
		return nil, fmt.Errorf("qq token exchange failed: %s %s", message, fields.Get("error_description"))
	}
	accessToken := fields.Get("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("qq access token is required")
	}
	expiresIn, _ := strconv.Atoi(fields.Get("expires_in"))
	return &oauth2.Token{
		AccessToken:  accessToken,
		TokenType:    "bearer",
		RefreshToken: fields.Get("refresh_token"),
		ExpiresIn:    int64(expiresIn),
		Expiry:       time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}

type qqMeResponse struct {
	OpenID      string `json:"openid"`
	AccessToken string `json:"access_token"`
}

type qqUserInfoResponse struct {
	Ret            int    `json:"ret"`
	Msg            string `json:"msg"`
	Nickname       string `json:"nickname"`
	Gender         string `json:"gender"`
	FigureURL      string `json:"figureurl"`
	FigureURLQQ100 string `json:"figureurl_qq_100"`
	FigureURLQQ240 string `json:"figureurl_qq_240"`
}

func (p *QQProvider) User(ctx context.Context, token *oauth2.Token, _ string) (*ExternalUser, error) {
	if token == nil || strings.TrimSpace(token.AccessToken) == "" {
		return nil, errors.New("qq access token is required")
	}
	meQ := url.Values{}
	meQ.Set("format", "json")
	meQ.Set("access_token", token.AccessToken)
	me, err := doGetJSON[qqMeResponse](ctx, p.httpClient, qqMeURL+"?"+meQ.Encode())
	if err != nil {
		return nil, err
	}
	openID := strings.TrimSpace(me.OpenID)
	if openID == "" {
		return nil, errors.New("qq openid is required")
	}
	infoQ := url.Values{}
	infoQ.Set("oauth_consumer_key", p.clientID)
	infoQ.Set("access_token", token.AccessToken)
	infoQ.Set("openid", openID)
	info, err := doGetJSON[qqUserInfoResponse](ctx, p.httpClient, qqUserInfoURL+"?"+infoQ.Encode())
	if err != nil {
		return nil, err
	}
	if info == nil || info.Ret != 0 {
		return nil, fmt.Errorf("qq userinfo failed: ret=%d msg=%s", info.Ret, info.Msg)
	}
	displayName := strings.TrimSpace(info.Nickname)
	if displayName == "" {
		displayName = openID
	}
	avatarURL := firstNonEmpty(info.FigureURLQQ240, info.FigureURLQQ100, info.FigureURL)
	return &ExternalUser{
		Provider:      QQ,
		IdentityID:    openID,
		Email:         syntheticEmail(QQ, openID),
		EmailVerified: true,
		Username:      displayName,
		Name:          displayName,
		AvatarURL:     strings.TrimSpace(avatarURL),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
