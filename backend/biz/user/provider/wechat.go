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
	wechatAuthorizeURL = "https://open.weixin.qq.com/connect/qrconnect"
	wechatTokenURL     = "https://api.weixin.qq.com/sns/oauth2/access_token"
	wechatUserInfoURL  = "https://api.weixin.qq.com/sns/userinfo"
)

// WechatProvider 对接微信开放平台扫码登录（snsapi_userinfo 授权）。
// 微信开放平台不返回邮箱，用 provider 身份派生只读邮箱兜底。
type WechatProvider struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	redirectURL  string
}

func NewWechat(cfg config.OAuthLoginProviderConfig, serverBaseURL string, httpClient *http.Client) *WechatProvider {
	return &WechatProvider{
		httpClient:   oauthClient(httpClient),
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		redirectURL:  oauthCallbackURL(cfg.RedirectURL, serverBaseURL, Wechat),
	}
}

func (p *WechatProvider) Name() Name {
	return Wechat
}

func (p *WechatProvider) AuthCodeURL(state, _ string) string {
	q := url.Values{}
	q.Set("appid", p.clientID)
	q.Set("redirect_uri", p.redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "snsapi_userinfo")
	q.Set("state", state)
	return wechatAuthorizeURL + "?" + q.Encode()
}

type wechatTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID       string `json:"openid"`
	UnionID      string `json:"unionid"`
	Scope        string `json:"scope"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

func (p *WechatProvider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	q := url.Values{}
	q.Set("appid", p.clientID)
	q.Set("secret", p.clientSecret)
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")
	body, err := doGetJSON[wechatTokenResponse](ctx, p.httpClient, wechatTokenURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	if body == nil || strings.TrimSpace(body.AccessToken) == "" {
		return nil, fmt.Errorf("wechat token exchange failed: code=%d msg=%s", body.ErrCode, body.ErrMsg)
	}
	token := &oauth2.Token{
		AccessToken:  body.AccessToken,
		TokenType:    "bearer",
		RefreshToken: body.RefreshToken,
		ExpiresIn:    int64(body.ExpiresIn),
		Expiry:       time.Now().Add(time.Duration(body.ExpiresIn) * time.Second),
	}
	extra := map[string]any{}
	if openID := strings.TrimSpace(body.OpenID); openID != "" {
		extra["openid"] = openID
	}
	if unionID := strings.TrimSpace(body.UnionID); unionID != "" {
		extra["unionid"] = unionID
	}
	return token.WithExtra(extra), nil
}

type wechatUserInfoResponse struct {
	OpenID     string `json:"openid"`
	Nickname   string `json:"nickname"`
	HeadImgURL string `json:"headimgurl"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

func (p *WechatProvider) User(ctx context.Context, token *oauth2.Token, _ string) (*ExternalUser, error) {
	if token == nil || strings.TrimSpace(token.AccessToken) == "" {
		return nil, errors.New("wechat access token is required")
	}
	openID := oauthTokenString(token, "openid")
	if openID == "" {
		return nil, errors.New("wechat openid is required")
	}
	q := url.Values{}
	q.Set("access_token", token.AccessToken)
	q.Set("openid", openID)
	info, err := doGetJSON[wechatUserInfoResponse](ctx, p.httpClient, wechatUserInfoURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	if info == nil || info.ErrCode != 0 {
		return nil, fmt.Errorf("wechat userinfo failed: code=%d msg=%s", info.ErrCode, info.ErrMsg)
	}
	displayName := strings.TrimSpace(info.Nickname)
	if displayName == "" {
		displayName = openID
	}
	return &ExternalUser{
		Provider:      Wechat,
		IdentityID:    openID,
		Email:         syntheticEmail(Wechat, openID),
		EmailVerified: true,
		Username:      displayName,
		Name:          displayName,
		AvatarURL:     strings.TrimSpace(info.HeadImgURL),
	}, nil
}

func oauthTokenString(token *oauth2.Token, key string) string {
	if token == nil {
		return ""
	}
	extra, ok := token.Extra(key).(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(extra)
}
