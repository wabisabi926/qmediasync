package fnosproxy

import (
	"Q115-STRM/internal/helpers"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// redirectSTRMStream 拦截视频流请求，302重定向到STRM中的直链
func (s *Service) redirectSTRMStream(w http.ResponseWriter, r *http.Request, cfg Config, fullPath string) {
	mediaSourceID := queryValue(r, "mediasourceid")
	itemID := ""
	if m := videoStreamPathRE.FindStringSubmatch(fullPath); len(m) > 1 {
		itemID = m[1]
	}
	if itemID == "" {
		itemID = extractItemID(fullPath)
	}

	playURL, strmPath := s.resolvePlayURL(mediaSourceID, itemID, cfg)
	if playURL == "" && itemID != "" {
		// 缓存未命中时从 Item 详情重新定位 STRM
		if path := s.fetchItemPath(r, cfg, itemID); isStrmPath(path) {
			strmPath = path
			if u := s.readStrmURL(path, cfg); u != "" {
				playURL = u
				s.rememberSource(mediaSourceID, itemID, path, u)
			}
		}
	}
	if playURL != "" {
		w.Header().Set("Location", playURL)
		w.WriteHeader(http.StatusFound)
		return
	}
	if strmPath != "" {
		helpers.AppLogger.Warnf("飞牛反代无法读取 strm，透传上游: path=%s, media_source_id=%s, item_id=%s", strmPath, mediaSourceID, itemID)
	}
	s.proxyRequest(w, r, cfg, fullPath)
}

func (s *Service) resolvePlayURL(mediaSourceID, itemID string, cfg Config) (playURL, strmPath string) {
	src := s.lookupCached(mediaSourceID, itemID)
	if src == nil {
		return "", ""
	}
	strmPath = src.Path
	if src.URL != "" {
		return src.URL, strmPath
	}
	if u := s.readStrmURL(src.Path, cfg); u != "" {
		s.rememberSource(mediaSourceID, itemID, src.Path, u)
		return u, strmPath
	}
	return "", strmPath
}

// redirectItemFile 拦截 Download/File 请求
func (s *Service) redirectItemFile(w http.ResponseWriter, r *http.Request, cfg Config, fullPath string) {
	itemID := ""
	if m := itemFilePathRE.FindStringSubmatch(fullPath); len(m) > 1 {
		itemID = m[1]
	}
	playURL, strmPath := s.resolvePlayURL("", itemID, cfg)
	if playURL == "" && itemID != "" {
		if path := s.fetchItemPath(r, cfg, itemID); isStrmPath(path) {
			strmPath = path
			playURL = s.readStrmURL(path, cfg)
			if playURL != "" {
				s.rememberSource("", itemID, path, playURL)
			}
		}
	}
	if playURL != "" {
		w.Header().Set("Location", playURL)
		w.WriteHeader(http.StatusFound)
		return
	}
	if strmPath != "" {
		helpers.AppLogger.Warnf("飞牛反代 Item 文件请求无法读 strm，透传上游: item_id=%s, path=%s", itemID, strmPath)
	}
	s.proxyRequest(w, r, cfg, fullPath)
}

// fetchItemDetail 拉取 Item 详情
func (s *Service) fetchItemDetail(r *http.Request, cfg Config, itemID string) map[string]any {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" || cfg.FnosURL == "" {
		return nil
	}
	q := url.Values{}
	q.Set("Fields", "Path,MediaSources")
	if tok := queryValue(r, "api_key"); tok != "" {
		q.Set("api_key", tok)
	}
	target, err := targetURL(cfg, withEmbyAPIPrefix("Items/"+itemID), q.Encode())
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		return nil
	}
	copyRequestHeaders(req.Header, r.Header, true)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode >= 400 {
		return nil
	}
	body = maybeGunzipBody(resp, body)
	if looksLikeHTML(body) {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	return payload
}

func (s *Service) fetchItemPath(r *http.Request, cfg Config, itemID string) string {
	payload := s.fetchItemDetail(r, cfg, itemID)
	if payload == nil {
		return ""
	}
	if path := strings.TrimSpace(stringValue(payload, "Path")); path != "" {
		return path
	}
	for _, ms := range mediaSources(payload) {
		if path := strings.TrimSpace(stringValue(ms, "Path")); isStrmPath(path) {
			return path
		}
	}
	return ""
}
