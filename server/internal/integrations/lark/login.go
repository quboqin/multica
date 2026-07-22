package lark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const defaultLoginRequestTimeout = 10 * time.Second

// LoginClientConfig configures the Feishu/Lark web-login OAuth client.
type LoginClientConfig struct {
	BaseURL    string
	HTTPClient *http.Client
	Logger     *slog.Logger
}

func (c LoginClientConfig) withDefaults(region Region) LoginClientConfig {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.BaseURL == "" {
		c.BaseURL = RegionOrDefault(string(region)).OpenPlatformBaseURL()
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultLoginRequestTimeout}
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// LoginUserInfo is the Multica-owned identity slice returned by Feishu/Lark
// after the backend exchanges a short-lived authorization code.
type LoginUserInfo struct {
	OpenID    string
	UnionID   string
	Name      string
	Email     string
	AvatarURL string
}

type LoginClient struct {
	cfg LoginClientConfig
}

func NewLoginClient(cfg LoginClientConfig, region Region) *LoginClient {
	return &LoginClient{cfg: cfg.withDefaults(region)}
}

func (c *LoginClient) ExchangeCode(ctx context.Context, appID, appSecret, code string) (LoginUserInfo, error) {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	code = strings.TrimSpace(code)
	if appID == "" || appSecret == "" || code == "" {
		return LoginUserInfo{}, errors.New("lark login: app_id, app_secret, and code are required")
	}

	appToken, err := c.appAccessToken(ctx, appID, appSecret)
	if err != nil {
		return LoginUserInfo{}, err
	}
	token, info, err := c.userAccessToken(ctx, appToken, code)
	if err != nil {
		return LoginUserInfo{}, err
	}
	if info.OpenID != "" && info.UnionID != "" {
		return info, nil
	}
	return c.userInfo(ctx, token)
}

func (c *LoginClient) appAccessToken(ctx context.Context, appID, appSecret string) (string, error) {
	body := map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	}
	var resp struct {
		Code           int    `json:"code"`
		Msg            string `json:"msg"`
		AppAccessToken string `json:"app_access_token"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/open-apis/auth/v3/app_access_token/internal", "", body, &resp); err != nil {
		return "", fmt.Errorf("lark login: app access token: %w", err)
	}
	if resp.Code != 0 || strings.TrimSpace(resp.AppAccessToken) == "" {
		return "", fmt.Errorf("lark login: app access token failed: code=%d msg=%q", resp.Code, resp.Msg)
	}
	return strings.TrimSpace(resp.AppAccessToken), nil
}

func (c *LoginClient) userAccessToken(ctx context.Context, appAccessToken, code string) (string, LoginUserInfo, error) {
	body := map[string]string{
		"grant_type": "authorization_code",
		"code":       code,
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			AccessToken string `json:"access_token"`
			OpenID      string `json:"open_id"`
			UnionID     string `json:"union_id"`
			Name        string `json:"name"`
			EnName      string `json:"en_name"`
			Email       string `json:"email"`
			AvatarURL   string `json:"avatar_url"`
		} `json:"data"`
		AccessToken string `json:"access_token"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/open-apis/authen/v1/access_token", appAccessToken, body, &resp); err != nil {
		return "", LoginUserInfo{}, fmt.Errorf("lark login: exchange code: %w", err)
	}
	token := strings.TrimSpace(resp.Data.AccessToken)
	if token == "" {
		token = strings.TrimSpace(resp.AccessToken)
	}
	if resp.Code != 0 || token == "" {
		return "", LoginUserInfo{}, fmt.Errorf("lark login: exchange code failed: code=%d msg=%q", resp.Code, resp.Msg)
	}
	info := normalizeLoginUserInfo(LoginUserInfo{
		OpenID:    resp.Data.OpenID,
		UnionID:   resp.Data.UnionID,
		Name:      firstNonEmpty(resp.Data.Name, resp.Data.EnName),
		Email:     resp.Data.Email,
		AvatarURL: resp.Data.AvatarURL,
	})
	return token, info, nil
}

func (c *LoginClient) userInfo(ctx context.Context, userAccessToken string) (LoginUserInfo, error) {
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID    string `json:"open_id"`
			UnionID   string `json:"union_id"`
			Name      string `json:"name"`
			EnName    string `json:"en_name"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
			Avatar    struct {
				AvatarURL string `json:"avatar_url"`
			} `json:"avatar"`
		} `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/open-apis/authen/v1/user_info", userAccessToken, nil, &resp); err != nil {
		return LoginUserInfo{}, fmt.Errorf("lark login: user info: %w", err)
	}
	if resp.Code != 0 {
		return LoginUserInfo{}, fmt.Errorf("lark login: user info failed: code=%d msg=%q", resp.Code, resp.Msg)
	}
	info := normalizeLoginUserInfo(LoginUserInfo{
		OpenID:    resp.Data.OpenID,
		UnionID:   resp.Data.UnionID,
		Name:      resp.Data.Name,
		Email:     resp.Data.Email,
		AvatarURL: resp.Data.AvatarURL,
	})
	if info.Name == "" {
		info.Name = strings.TrimSpace(resp.Data.EnName)
	}
	if info.AvatarURL == "" {
		info.AvatarURL = strings.TrimSpace(resp.Data.Avatar.AvatarURL)
	}
	if info.UnionID == "" {
		return LoginUserInfo{}, errors.New("lark login: user info missing union_id")
	}
	if info.OpenID == "" {
		return LoginUserInfo{}, errors.New("lark login: user info missing open_id")
	}
	return info, nil
}

func normalizeLoginUserInfo(info LoginUserInfo) LoginUserInfo {
	info.OpenID = strings.TrimSpace(info.OpenID)
	info.UnionID = strings.TrimSpace(info.UnionID)
	info.Name = strings.TrimSpace(info.Name)
	info.Email = strings.ToLower(strings.TrimSpace(info.Email))
	info.AvatarURL = strings.TrimSpace(info.AvatarURL)
	return info
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (c *LoginClient) doJSON(ctx context.Context, method, path, bearer string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", res.StatusCode, string(data))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}
