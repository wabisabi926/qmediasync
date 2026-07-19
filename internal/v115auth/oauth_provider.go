package v115auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/v115open"

	"resty.dev/v3"
)

type OAuthURLRequest struct {
	AccountID   uint
	AppID       string
	RedirectURL string
	Provider    AuthProvider
}

type OAuthURLResult struct {
	AuthURL   string `json:"auth_url,omitempty"`
	State     string `json:"state,omitempty"`
	Polling   bool   `json:"polling"`
	ExpiresIn int64  `json:"expires_in,omitempty"`
}

type OAuthTokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	Done         bool
}

type OAuthProvider interface {
	BuildAuth(ctx context.Context, req OAuthURLRequest) (OAuthURLResult, error)
	Confirm(ctx context.Context, payload map[string]string) (OAuthTokenResult, error)
	Poll(ctx context.Context, state string) (OAuthTokenResult, error)
}

var errUnsupportedOAuthOperation = fmt.Errorf("当前授权服务不支持此操作")

func GetOAuthProvider(provider AuthProvider) (OAuthProvider, bool) {
	switch provider {
	case AuthProviderQMediaSync, AuthProviderMQFamily:
		return relayOAuthProvider{}, true
	case AuthProviderMoviePilot:
		return moviePilotOAuthProvider{authServer: "https://movie-pilot.org", client: defaultOAuthHTTPClient()}, true
	case AuthProviderCloudDrive:
		return cloudDriveOAuthProvider{}, true
	default:
		return nil, false
	}
}

func defaultOAuthHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

type relayOAuthProvider struct{}

func (provider relayOAuthProvider) BuildAuth(_ context.Context, req OAuthURLRequest) (OAuthURLResult, error) {
	clientID := strings.TrimSpace(req.AppID)
	if clientID == "" {
		clientID = "100197665"
		if req.Provider == AuthProviderQMediaSync {
			clientID = "100197849"
		}
	}
	redirectURL := strings.TrimSpace(req.RedirectURL)
	if redirectURL != "" {
		redirectURL = fmt.Sprintf("%s?source=115", redirectURL)
	}
	stateObj := struct {
		State       string `json:"state"`
		Time        int64  `json:"time"`
		ClientId    string `json:"client_id"`
		RedirectUrl string `json:"redirect_url"`
		AccountId   uint   `json:"account_id"`
	}{
		State:       helpers.RandStr(16),
		Time:        time.Now().Unix(),
		ClientId:    clientID,
		RedirectUrl: redirectURL,
		AccountId:   req.AccountID,
	}
	stateJSON, _ := json.Marshal(stateObj)
	stateEncoded, err := helpers.Encrypt(string(stateJSON))
	if err != nil {
		return OAuthURLResult{}, err
	}
	baseURL := helpers.GlobalConfig.AuthServer
	if req.Provider == AuthProviderQMediaSync || clientID == "100197849" {
		baseURL = helpers.GlobalConfig.NewAuthServer
	}
	return OAuthURLResult{AuthURL: fmt.Sprintf("%s/115.php?action=code&state=%s", strings.TrimRight(baseURL, "/"), stateEncoded)}, nil
}

func (provider relayOAuthProvider) Confirm(_ context.Context, payload map[string]string) (OAuthTokenResult, error) {
	data := payload["data"]
	if data == "" {
		return OAuthTokenResult{}, fmt.Errorf("缺少中转回调数据")
	}
	decryptedData, err := helpers.Decrypt(data)
	if err != nil {
		return OAuthTokenResult{}, err
	}
	var resp struct {
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int64  `json:"expires_in"`
		} `json:"data"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(decryptedData), &resp); err != nil {
		return OAuthTokenResult{}, err
	}
	if resp.Data.AccessToken == "" || resp.Data.RefreshToken == "" {
		if resp.Error != "" {
			return OAuthTokenResult{}, fmt.Errorf(resp.Error)
		}
		if resp.Message != "" {
			return OAuthTokenResult{}, fmt.Errorf(resp.Message)
		}
		return OAuthTokenResult{}, fmt.Errorf("中转回调未返回访问凭证")
	}
	return OAuthTokenResult{AccessToken: resp.Data.AccessToken, RefreshToken: resp.Data.RefreshToken, ExpiresIn: resp.Data.ExpiresIn, Done: true}, nil
}

func (provider relayOAuthProvider) Poll(_ context.Context, _ string) (OAuthTokenResult, error) {
	return OAuthTokenResult{}, errUnsupportedOAuthOperation
}

type moviePilotOAuthProvider struct {
	authServer string
	client     *http.Client
}

func (provider moviePilotOAuthProvider) BuildAuth(ctx context.Context, req OAuthURLRequest) (OAuthURLResult, error) {
	endpoint := strings.TrimRight(provider.authServer, "/") + "/u115/auth_url"
	resp, err := httpGetJSON(ctx, provider.client, endpoint)
	if err != nil {
		return OAuthURLResult{}, err
	}
	authURL := stringField(resp, "auth_url")
	state := stringField(resp, "state")
	if authURL == "" || state == "" {
		return OAuthURLResult{}, fmt.Errorf("MoviePilot 授权服务响应缺少 auth_url 或 state")
	}
	GlobalAuthStateManager.SetOAuthState(state, strconv.FormatUint(uint64(req.AccountID), 10), AuthProviderMoviePilot, req.RedirectURL)
	return OAuthURLResult{AuthURL: authURL, State: state, Polling: true, ExpiresIn: 300}, nil
}

func (provider moviePilotOAuthProvider) Confirm(_ context.Context, _ map[string]string) (OAuthTokenResult, error) {
	return OAuthTokenResult{}, errUnsupportedOAuthOperation
}

func (provider moviePilotOAuthProvider) Poll(ctx context.Context, state string) (OAuthTokenResult, error) {
	if _, ok := GlobalAuthStateManager.GetOAuthStateWithProvider(state, AuthProviderMoviePilot); !ok {
		return OAuthTokenResult{}, fmt.Errorf("授权状态不存在或已过期")
	}
	endpoint := strings.TrimRight(provider.authServer, "/") + "/u115/token?state=" + url.QueryEscape(state)
	resp, err := httpGetJSON(ctx, provider.client, endpoint)
	if err != nil {
		return OAuthTokenResult{}, err
	}
	token := tokenResultFromMap(resp)
	if !token.Done {
		return OAuthTokenResult{Done: false}, nil
	}
	GlobalAuthStateManager.DeleteOAuthState(state)
	return token, nil
}

type cloudDriveOAuthProvider struct{}

func (provider cloudDriveOAuthProvider) BuildAuth(_ context.Context, req OAuthURLRequest) (OAuthURLResult, error) {
	state, err := cloudDriveCallbackState(req.RedirectURL, req.AccountID)
	if err != nil {
		return OAuthURLResult{}, err
	}
	clientID := strings.TrimSpace(req.AppID)
	if clientID == "" {
		clientID = "100195313"
	}
	authURL, _ := url.Parse("https://passportapi.115.com/open/authorize")
	query := authURL.Query()
	query.Set("client_id", clientID)
	query.Set("redirect_uri", "https://redirect115.zhenyunpan.com")
	query.Set("response_type", "code")
	query.Set("state", state)
	authURL.RawQuery = query.Encode()
	return OAuthURLResult{AuthURL: authURL.String(), State: state, ExpiresIn: 300}, nil
}

func (provider cloudDriveOAuthProvider) Confirm(_ context.Context, payload map[string]string) (OAuthTokenResult, error) {
	token := tokenResultFromPayload(payload)
	if !token.Done {
		return OAuthTokenResult{}, fmt.Errorf("CloudDrive 回调未返回访问凭证")
	}
	return token, nil
}

func (provider cloudDriveOAuthProvider) Poll(_ context.Context, _ string) (OAuthTokenResult, error) {
	return OAuthTokenResult{}, errUnsupportedOAuthOperation
}

func cloudDriveCallbackState(redirectURL string, accountID uint) (string, error) {
	redirectURL = strings.TrimSpace(redirectURL)
	if redirectURL == "" {
		return "", fmt.Errorf("CloudDrive 授权缺少回跳地址")
	}
	callbackURL, err := url.Parse(redirectURL)
	if err != nil || callbackURL.Scheme == "" || callbackURL.Host == "" {
		return "", fmt.Errorf("CloudDrive 授权回跳地址无效")
	}
	values := url.Values{}
	values.Set("source", "115")
	values.Set("account_id", strconv.FormatUint(uint64(accountID), 10))
	if callbackURL.Fragment != "" {
		fragmentPath, fragmentQuery, hasQuery := strings.Cut(callbackURL.Fragment, "?")
		if hasQuery {
			values, _ = url.ParseQuery(fragmentQuery)
			values.Set("source", "115")
			values.Set("account_id", strconv.FormatUint(uint64(accountID), 10))
		}
		callbackURL.Fragment = fragmentPath + "?" + values.Encode()
		return callbackURL.String(), nil
	}
	query := callbackURL.Query()
	query.Set("source", "115")
	query.Set("account_id", strconv.FormatUint(uint64(accountID), 10))
	callbackURL.RawQuery = query.Encode()
	return callbackURL.String(), nil
}

func httpGetJSON(ctx context.Context, client *http.Client, endpoint string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("授权服务返回 HTTP %d：%s", resp.StatusCode, string(body))
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if nested, ok := data["data"].(map[string]any); ok {
		for key, value := range nested {
			if _, exists := data[key]; !exists {
				data[key] = value
			}
		}
	}
	return data, nil
}

func tokenResultFromPayload(payload map[string]string) OAuthTokenResult {
	expiresIn, _ := strconv.ParseInt(payload["expires_in"], 10, 64)
	token := OAuthTokenResult{
		AccessToken:  payload["access_token"],
		RefreshToken: payload["refresh_token"],
		ExpiresIn:    expiresIn,
	}
	token.Done = token.AccessToken != "" && token.RefreshToken != ""
	return token
}

func tokenResultFromMap(data map[string]any) OAuthTokenResult {
	if nested, ok := data["data"].(map[string]any); ok {
		for key, value := range nested {
			if _, exists := data[key]; !exists {
				data[key] = value
			}
		}
	}
	expiresIn := int64(0)
	switch value := data["expires_in"].(type) {
	case float64:
		expiresIn = int64(value)
	case json.Number:
		expiresIn, _ = value.Int64()
	case string:
		expiresIn, _ = strconv.ParseInt(value, 10, 64)
	}
	token := OAuthTokenResult{
		AccessToken:  stringField(data, "access_token"),
		RefreshToken: stringField(data, "refresh_token"),
		ExpiresIn:    expiresIn,
	}
	token.Done = token.AccessToken != "" && token.RefreshToken != ""
	return token
}

func stringField(data map[string]any, key string) string {
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func GenerateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func PKCEAuth(ctx context.Context, appID, appIDName string) (*v115open.QrCodeDataReturn, error) {
	codeVerifier := helpers.RandStr(64)
	data := make(map[string]string)
	data["client_id"] = appID
	data["code_challenge"] = v115open.GenCodeChallenge(codeVerifier)
	data["code_challenge_method"] = "sha256"

	client := resty.New()
	resp, err := client.R().
		SetContext(ctx).
		SetFormData(data).
		Post("https://passportapi.115.com/open/authDeviceCode")

	if err != nil {
		return nil, err
	}

	var result v115open.RespBase[v115open.QrCodeData]
	if err := json.Unmarshal([]byte(resp.String()), &result); err != nil {
		return nil, err
	}

	if result.State != 1 {
		return nil, fmt.Errorf("获取开放平台授权二维码失败")
	}

	return &v115open.QrCodeDataReturn{
		QrCodeData:   result.Data,
		CodeVerifier: codeVerifier,
	}, nil
}

func PKCEExchangeToken(ctx context.Context, uid, codeVerifier string) (*v115open.TokenData, error) {
	data := make(map[string]string)
	data["uid"] = uid
	data["code_verifier"] = codeVerifier

	client := resty.New()
	resp, err := client.R().
		SetContext(ctx).
		SetFormData(data).
		Post("https://passportapi.115.com/open/deviceCodeToToken")

	if err != nil {
		return nil, err
	}

	var result v115open.RespBase[v115open.TokenData]
	if err := json.Unmarshal([]byte(resp.String()), &result); err != nil {
		return nil, err
	}

	if result.State != 1 {
		return nil, fmt.Errorf("获取访问凭证失败")
	}

	return &result.Data, nil
}

func PKCEPollStatus(ctx context.Context, uid, timeStr, sign string) (*v115open.QrCodeStatus, error) {
	data := make(map[string]string)
	data["uid"] = uid
	data["time"] = timeStr
	data["sign"] = sign

	client := resty.New()
	resp, err := client.R().
		SetContext(ctx).
		SetQueryParams(data).
		Get("https://qrcodeapi.115.com/get/status/")

	if err != nil {
		return nil, err
	}

	var result v115open.RespBase[v115open.QrCodeStatus]
	if err := json.Unmarshal([]byte(resp.String()), &result); err != nil {
		return nil, err
	}

	if result.State != 1 {
		return nil, fmt.Errorf("获取二维码状态失败")
	}

	return &result.Data, nil
}

func GetPKCEAuthURL(appID string) string {
	return fmt.Sprintf("https://open.115.com/device/authorize/%s", appID)
}

func BuildOAuthCallbackURL(baseURL, state, accountID string) string {
	parsed, _ := url.Parse(baseURL)
	params := url.Values{}
	params.Set("state", state)
	params.Set("account_id", accountID)
	parsed.RawQuery = params.Encode()
	return parsed.String()
}

func ParseOAuthCallbackURL(callbackURL string) (state, accountID, code string, err error) {
	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return
	}

	state = parsed.Query().Get("state")
	accountID = parsed.Query().Get("account_id")
	code = parsed.Query().Get("code")

	return
}

func ParseOAuthCodeFromURL(callbackURL string) string {
	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("code")
}

func GeneratePollID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func ExtractAccountIDFromState(state string) (string, error) {
	parts := strings.SplitN(state, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid state format")
	}
	return parts[1], nil
}

func BuildStateWithAccountID(accountID string) string {
	nonce := GenerateState()[:16]
	return fmt.Sprintf("%s:%s", nonce, accountID)
}
