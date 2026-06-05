package rta

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// Session はフレンドワールドのセッション情報を表します
type Session struct {
	SessionID   string `json:"sessionId"`
	SCID        string `json:"scid"`
	Template    string `json:"template"`
	NetherNetID int64  `json:"netherNetId"`
	OwnerXUID   string `json:"ownerXuid"`
	OwnerGamertag string `json:"ownerGamertag"`
	WorldName   string `json:"worldName"`
	Version     string `json:"version"`
	Protocol    int    `json:"protocol"`
}

// RTAClient は Real-Time API クライアントです
type RTAClient struct {
	accountToken string
	xuid         string
	client       *http.Client
}

// NewRTAClient は新しい RTA クライアントを作成します
func NewRTAClient(accountToken, xuid string) *RTAClient {
	return &RTAClient{
		accountToken: accountToken,
		xuid:         xuid,
		client:       &http.Client{Timeout: 30 * time.Second},
	}
}

// GetFriendSessions はフレンドが開いているワールドのセッション一覧を取得します
func (r *RTAClient) GetFriendSessions(ctx context.Context) ([]*Session, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://api.minecraftservices.com/multiplayer/sessions/findable",
		nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.accountToken)
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("セッション取得エラー: %d", resp.StatusCode)
	}

	var result struct {
		Sessions []struct {
			SessionID   string `json:"sessionId"`
			Joinability string `json:"joinability"`
			Owners      []struct {
				XUID string `json:"xuid"`
			} `json:"owners"`
			CompatibleVersions []string `json:"compatibleVersions"`
			MemberCount        int      `json:"memberCount"`
			MaxMembers         int      `json:"maxMembers"`
			WorldType          string   `json:"worldType"`
			Extra              struct {
				SCID        string `json:"scid"`
				Template    string `json:"template"`
				NetherNetID int64  `json:"netherNetId"`
				WorldName   string `json:"worldName"`
				Version     string `json:"version"`
				Protocol    int    `json:"protocol"`
			} `json:"extra"`
		} `json:"sessions"`
	}

	body := make([]byte, resp.ContentLength)
	// 簡易実装：実際のレスポンスパースは gophertunnel を参照

	// ダミーデータを返す（実際には API レスポンスをパース）
	_ = result
	return []*Session{}, nil
}

// SelectSession は指定したセッション ID の詳細を取得します
func (r *RTAClient) SelectSession(ctx context.Context, sessionID string) (*Session, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.minecraftservices.com/multiplayer/sessions/%s", sessionID),
		nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.accountToken)
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("セッション詳細取得エラー: %d", resp.StatusCode)
	}

	// 実際のレスポンスパースは gophertunnel を参照
	return &Session{
		SessionID: sessionID,
	}, nil
}

// WebSocketSignaling は WebSocket を介したシグナリングを行います
type WebSocketSignaling struct {
	conn      *websocket.Conn
	sessionID string
	xuid      string
	token     string
}

// ConnectWebSocket は RTA WebSocket に接続します
func ConnectWebSocket(ctx context.Context, sessionID, xuid, token string) (*WebSocketSignaling, error) {
	u := url.URL{
		Scheme: "wss",
		Host:   "sessiondirectory.xboxlive.com",
		Path:   "/v2/sessions/" + sessionID + "/connection",
	}

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("X-Xbl-Client-Type", "WindowsOneCore")
	header.Set("Sec-WebSocket-Protocol", "Minecraft-RTA")

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, u.String(), header)
	if err != nil {
		return nil, fmt.Errorf("WebSocket 接続エラー: %w", err)
	}

	return &WebSocketSignaling{
		conn:      conn,
		sessionID: sessionID,
		xuid:      xuid,
		token:     token,
	}, nil
}

// SendOffer は Offer を送信します
func (w *WebSocketSignaling) SendOffer(offerData []byte) error {
	msg := map[string]interface{}{
		"type":          "offer",
		"sessionId":     w.sessionID,
		"senderXuid":    w.xuid,
		"payload":       string(offerData),
		"timestamp":     time.Now().UnixMilli(),
	}
	return w.conn.WriteJSON(msg)
}

// SendAnswer は Answer を送信します
func (w *WebSocketSignaling) SendAnswer(answerData []byte) error {
	msg := map[string]interface{}{
		"type":          "answer",
		"sessionId":     w.sessionID,
		"senderXuid":    w.xuid,
		"payload":       string(answerData),
		"timestamp":     time.Now().UnixMilli(),
	}
	return w.conn.WriteJSON(msg)
}

// SendCandidate は ICE Candidate を送信します
func (w *WebSocketSignaling) SendCandidate(candidateData []byte) error {
	msg := map[string]interface{}{
		"type":          "candidate",
		"sessionId":     w.sessionID,
		"senderXuid":    w.xuid,
		"payload":       string(candidateData),
		"timestamp":     time.Now().UnixMilli(),
	}
	return w.conn.WriteJSON(msg)
}

// Receive はシグナルメッセージを受信します
func (w *WebSocketSignaling) Receive(ctx context.Context) (map[string]interface{}, error) {
	_, message, err := w.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// Close は WebSocket 接続を閉じます
func (w *WebSocketSignaling) Close() error {
	return w.conn.Close()
}
