package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Account は Minecraft アカウント情報を表します
type Account struct {
	XUID        string `json:"xuid"`
	Gamertag    string `json:"gamertag"`
	TitleID     string `json:"titleId"`
	Token       string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// Authenticator は Xbox Live 認証を行います
type Authenticator struct {
	client *http.Client
}

// NewAuthenticator は新しい Authenticator を作成します
func NewAuthenticator() *Authenticator {
	return &Authenticator{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// DeviceCodeAuthFlow は microsoft.com/link を使用したデバイスコード認証フローを実行します
func (a *Authenticator) DeviceCodeAuthFlow(ctx context.Context) (*Account, error) {
	// 1. デバイスコードを取得
	deviceCode, err := a.getDeviceCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("デバイスコードの取得に失敗: %w", err)
	}

	fmt.Printf("\n========================================\n")
	fmt.Printf("Minecraft にログインしてください\n")
	fmt.Printf("========================================\n")
	fmt.Printf("URL: https://microsoft.com/link\n")
	fmt.Printf("コード: %s\n", deviceCode.UserCode)
	fmt.Printf("\nブラウザで上記 URL を開き、コードを入力してください...\n")
	fmt.Printf("========================================\n\n")

	// 2. 認証完了をポーリング
	account, err := a.pollForToken(ctx, deviceCode)
	if err != nil {
		return nil, fmt.Errorf("認証待機に失敗: %w", err)
	}

	fmt.Printf("✅ ログイン成功！ Gamertag: %s (XUID: %s)\n\n", account.Gamertag, account.XUID)
	return account, nil
}

type deviceCodeResponse struct {
	DeviceCode  string `json:"device_code"`
	UserCode    string `json:"user_code"`
	VerificationURL string `json:"verification_uri"`
	ExpiresIn   int    `json:"expires_in"`
	Interval    int    `json:"interval"`
	Message     string `json:"message"`
}

func (a *Authenticator) getDeviceCode(ctx context.Context) (*deviceCodeResponse, error) {
	data := url.Values{}
	data.Set("client_id", "000000004c12ae6f") // Minecraft のクライアント ID
	data.Set("scope", "service::user.auth.xboxlive.com::MBI_SSL")
	data.Set("response_type", "device_code")

	req, err := http.NewRequestWithContext(ctx, "POST", 
		"https://login.live.com/oauth20_connect.srf", 
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("デバイスコード取得エラー: %s", string(body))
	}

	var dc deviceCodeResponse
	if err := json.Unmarshal(body, &dc); err != nil {
		return nil, err
	}

	return &dc, nil
}

type tokenResponse struct {
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
}

func (a *Authenticator) pollForToken(ctx context.Context, deviceCode *deviceCodeResponse) (*Account, error) {
	interval := time.Duration(deviceCode.Interval) * time.Second
	if interval < 1*time.Second {
		interval = 5 * time.Second // デフォルト間隔を確保
	}
	timeout := time.Duration(deviceCode.ExpiresIn) * time.Second
	if timeout < 1*time.Minute {
		timeout = 15 * time.Minute // デフォルトタイムアウトを確保
	}
	
	fmt.Printf("🔄 認証待機中... (間隔: %v, タイムアウト: %v)\n", interval, timeout)
	
	timer := time.NewTimer(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer timer.Stop()

	data := url.Values{}
	data.Set("client_id", "000000004c12ae6f")
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	data.Set("device_code", deviceCode.DeviceCode)
	data.Set("scope", "service::user.auth.xboxlive.com::MBI_SSL")

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, fmt.Errorf("認証タイムアウト (%v)", timeout)
		case <-ticker.C:
			attempt++
			if attempt%10 == 0 {
				fmt.Printf("⏳ 待機中... (%d/%d 分)\n", attempt/12, int(timeout.Minutes()))
			}
			account, err := a.tryGetToken(ctx, data)
			if err == nil {
				return account, nil
			}
			// デバッグ用に最初の数回と最後のエラーを出力
			if attempt <= 3 || attempt%30 == 0 {
				fmt.Printf("📡 認証確認中... (試行: %d) エラー: %v\n", attempt, err)
			}
		}
	}
}

func (a *Authenticator) tryGetToken(ctx context.Context, data url.Values) (*Account, error) {
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://login.live.com/oauth20_token.srf",
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("トークン取得エラー: %s", string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, err
	}

	// XBL トークンを取得
	xblToken, userHash, err := a.getXBLToken(ctx, tr.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("XBL トークン取得エラー: %w", err)
	}

	// XSTS トークンを取得
	xstsToken, err := a.getXSTSToken(ctx, xblToken)
	if err != nil {
		return nil, fmt.Errorf("XSTS トークン取得エラー: %w", err)
	}

	// Minecraft トークンを取得
	mcToken, err := a.getMinecraftToken(ctx, xstsToken, userHash)
	if err != nil {
		return nil, fmt.Errorf("Minecraft トークン取得エラー: %w", err)
	}

	return &Account{
		XUID:         mcToken.XUID,
		Gamertag:     mcToken.Gamertag,
		TitleID:      "000000004c12ae6f",
		Token:        mcToken.Token,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}

type xblResponse struct {
	Token       string `json:"Token"`
	DisplayClaims struct {
		XUI []struct {
			UHS string `json:"uhs"`
			Gamertag string `json:"gtg"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
}

func (a *Authenticator) getXBLToken(ctx context.Context, accessToken string) (string, string, error) {
	payload := map[string]interface{}{
		"Properties": map[string]interface{}{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  "d=" + accessToken,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", 
		"https://user.auth.xboxlive.com/user/authenticate", 
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("XBL エラー: %s", string(respBody))
	}

	var xbl xblResponse
	json.Unmarshal(respBody, &xbl)

	if len(xbl.DisplayClaims.XUI) == 0 {
		return "", "", fmt.Errorf("XUI クレームが見つかりません")
	}

	return xbl.Token, xbl.DisplayClaims.XUI[0].UHS, nil
}

func (a *Authenticator) getXSTSToken(ctx context.Context, xblToken string) (string, error) {
	payload := map[string]interface{}{
		"Properties": map[string]interface{}{
			"SandboxId": "RETAIL",
			"UserTokens": []string{xblToken},
		},
		"RelyingParty": "rp://api.minecraftservices.com/",
		"TokenType":    "JWT",
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://xsts.auth.xboxlive.com/xsts/authorize",
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("XSTS エラー: %s", string(respBody))
	}

	var xsts xblResponse
	json.Unmarshal(respBody, &xsts)

	return xsts.Token, nil
}

type minecraftTokenResponse struct {
	Username     string `json:"username"`
	UsernameUUID string `json:"username_uuid"`
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type minecraftAccount struct {
	XUID     string
	Gamertag string
	Token    string
}

func (a *Authenticator) getMinecraftToken(ctx context.Context, xstsToken, userHash string) (*minecraftAccount, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://api.minecraftservices.com/authentication/login_with_xbox",
		strings.NewReader(fmt.Sprintf(`{"identityToken": "XBL3.0 x=%s;%s"}`, userHash, xstsToken)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Minecraft ログインエラー: %s", string(respBody))
	}

	var mtr minecraftTokenResponse
	json.Unmarshal(respBody, &mtr)

	// UUID から XUID を抽出（ダッシュを削除）
	xuid := strings.ReplaceAll(mtr.UsernameUUID, "-", "")

	return &minecraftAccount{
		XUID:     xuid,
		Gamertag: mtr.Username,
		Token:    mtr.AccessToken,
	}, nil
}

// GenerateClientID はランダムなクライアント ID を生成します
func GenerateClientID() string {
	return uuid.New().String()
}
