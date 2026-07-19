package v115auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"Q115-STRM/internal/helpers"
)

type AuthSourceType string

const (
	AuthSourceTypeBuiltInAppID     AuthSourceType = "built_in_appid"
	AuthSourceTypeBuiltInRelay     AuthSourceType = "built_in_relay"
	AuthSourceTypeThirdPartyService AuthSourceType = "third_party_service"
	AuthSourceTypeCustomAppID      AuthSourceType = "custom_appid"
)

type AuthProvider string

const (
	AuthProviderOfficialPKCE AuthProvider = "official_pkce"
	AuthProviderMQFamily    AuthProvider = "mqfamily"
	AuthProviderQMediaSync  AuthProvider = "qmediasync"
	AuthProviderMoviePilot  AuthProvider = "moviepilot"
	AuthProviderCloudDrive  AuthProvider = "clouddrive"
)

type BuiltInAppIDItem struct {
	AppID       string          `json:"app_id"`
	AppName     string          `json:"app_name"`
	DisplayName string          `json:"display_name"`
	Provider    AuthProvider    `json:"provider"`
}

var (
	pinnedBuiltInAppIDs = []BuiltInAppIDItem{
		{AppID: "100197849", AppName: "QMediaSync", DisplayName: "QMediaSync", Provider: AuthProviderQMediaSync},
		{AppID: "100197665", AppName: "Q115-STRM", DisplayName: "Q115-STRM", Provider: AuthProviderMQFamily},
	}

	featuredBuiltInAppIDs = []BuiltInAppIDItem{
		{AppID: "100197847", AppName: "MoviePilot-115", DisplayName: "MoviePilot-115", Provider: AuthProviderOfficialPKCE},
		{AppID: "100197303", AppName: "OpenList", DisplayName: "OpenList", Provider: AuthProviderOfficialPKCE},
		{AppID: "100195313", AppName: "CloudDrive", DisplayName: "CloudDrive", Provider: AuthProviderOfficialPKCE},
		{AppID: "100195125", AppName: "媒体播放器", DisplayName: "媒体播放器", Provider: AuthProviderOfficialPKCE},
	}

	BuiltInAppIDMap = map[string]BuiltInAppIDItem{}

	remoteAppIDCache struct {
		sync.Mutex
		data []BuiltInAppIDItem
	}
)

func init() {
	for _, item := range append(pinnedBuiltInAppIDs, featuredBuiltInAppIDs...) {
		BuiltInAppIDMap[item.AppID] = item
	}
}

func GetPinnedBuiltInAppIDs() []BuiltInAppIDItem {
	return pinnedBuiltInAppIDs
}

func GetFeaturedBuiltInAppIDs() []BuiltInAppIDItem {
	return featuredBuiltInAppIDs
}

func GetAllBuiltInAppIDs() []BuiltInAppIDItem {
	return append(pinnedBuiltInAppIDs, featuredBuiltInAppIDs...)
}

func GetBuiltInAppID(appID string) (BuiltInAppIDItem, bool) {
	item, ok := BuiltInAppIDMap[appID]
	return item, ok
}

func FetchRemoteAppIDs(keyword string, offset, limit int) ([]BuiltInAppIDItem, int, error) {
	if helpers.GlobalConfig.NewAuthServer == "" {
		return nil, 0, nil
	}

	url := fmt.Sprintf("%s/api/appid/list?keyword=%s&offset=%d&limit=%d", helpers.GlobalConfig.NewAuthServer, keyword, offset, limit)

	resp, err := http.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("remote server returned status %d", resp.StatusCode)
	}

	var result struct {
		Code int               `json:"code"`
		Data struct {
			Items []BuiltInAppIDItem `json:"items"`
			Total int                `json:"total"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, err
	}

	if result.Code != 200 {
		return nil, 0, fmt.Errorf("remote server returned error code %d", result.Code)
	}

	return result.Data.Items, result.Data.Total, nil
}

type AuthSource struct {
	Type       AuthSourceType
	Provider   AuthProvider
	AppID      string
	AppIDName  string
	CustomName string
}

func ParseAuthSource(authSourceType, authProvider, appID, appIDName string) AuthSource {
	if authSourceType == "" && authProvider == "" {
		return parseLegacyAuthSource(appID, appIDName)
	}

	return AuthSource{
		Type:      AuthSourceType(authSourceType),
		Provider:  AuthProvider(authProvider),
		AppID:     appID,
		AppIDName: appIDName,
	}
}

func parseLegacyAuthSource(appID, appIDName string) AuthSource {
	if appID != "" {
		if item, ok := GetBuiltInAppID(appID); ok {
			return AuthSource{
				Type:      AuthSourceTypeBuiltInAppID,
				Provider:  item.Provider,
				AppID:     appID,
				AppIDName: item.DisplayName,
			}
		}
		return AuthSource{
			Type:      AuthSourceTypeCustomAppID,
			Provider:  AuthProviderOfficialPKCE,
			AppID:     appID,
			AppIDName: appIDName,
		}
	}

	switch strings.TrimSpace(appIDName) {
	case "QMediaSync":
		return AuthSource{
			Type:      AuthSourceTypeBuiltInRelay,
			Provider:  AuthProviderQMediaSync,
			AppID:     "100197849",
			AppIDName: "QMediaSync",
		}
	case "Q115-STRM":
		return AuthSource{
			Type:      AuthSourceTypeBuiltInRelay,
			Provider:  AuthProviderMQFamily,
			AppID:     appID,
			AppIDName: appIDName,
		}
	default:
		return AuthSource{
			Type:      AuthSourceTypeBuiltInRelay,
			Provider:  AuthProviderQMediaSync,
			AppIDName: "QMediaSync",
		}
	}
}

func (s AuthSource) SupportsPKCE() bool {
	return s.Type == AuthSourceTypeBuiltInAppID || s.Type == AuthSourceTypeCustomAppID
}

func (s AuthSource) SupportsOAuth() bool {
	return s.Type == AuthSourceTypeBuiltInRelay || s.Type == AuthSourceTypeThirdPartyService
}

func SearchAppIDs(keyword string, offset, limit int) ([]BuiltInAppIDItem, int, error) {
	allBuiltIn := GetAllBuiltInAppIDs()

	if keyword == "" {
		total := len(allBuiltIn)
		end := offset + limit
		if end > total {
			end = total
		}
		if offset >= total {
			return []BuiltInAppIDItem{}, total, nil
		}
		return allBuiltIn[offset:end], total, nil
	}

	lowerKeyword := strings.ToLower(keyword)
	var filtered []BuiltInAppIDItem
	for _, item := range allBuiltIn {
		if strings.Contains(strings.ToLower(item.AppName), lowerKeyword) ||
			strings.Contains(strings.ToLower(item.DisplayName), lowerKeyword) ||
			strings.Contains(item.AppID, keyword) {
			filtered = append(filtered, item)
		}
	}

	remoteItems, remoteTotal, err := FetchRemoteAppIDs(keyword, 0, limit)
	if err != nil {
		helpers.AppLogger.Warnf("Failed to fetch remote app IDs: %v", err)
	} else {
		seen := make(map[string]bool)
		for _, item := range filtered {
			seen[item.AppID] = true
		}
		for _, item := range remoteItems {
			if !seen[item.AppID] {
				filtered = append(filtered, item)
				seen[item.AppID] = true
			}
		}
		if remoteTotal > len(filtered) {
			return filtered, remoteTotal, nil
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		if strings.Contains(strings.ToLower(filtered[i].AppName), lowerKeyword) &&
			!strings.Contains(strings.ToLower(filtered[j].AppName), lowerKeyword) {
			return true
		}
		if !strings.Contains(strings.ToLower(filtered[i].AppName), lowerKeyword) &&
			strings.Contains(strings.ToLower(filtered[j].AppName), lowerKeyword) {
			return false
		}
		return filtered[i].AppName < filtered[j].AppName
	})

	total := len(filtered)
	end := offset + limit
	if end > total {
		end = total
	}
	if offset >= total {
		return []BuiltInAppIDItem{}, total, nil
	}
	return filtered[offset:end], total, nil
}