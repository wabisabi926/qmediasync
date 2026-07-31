package fnosproxy

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// === 工具函数 ===

func targetURL(cfg Config, fullPath, rawQuery string) (string, error) {
	baseStr := strings.TrimRight(strings.TrimSpace(cfg.FnosURL), "/")
	path := strings.TrimPrefix(strings.TrimSpace(fullPath), "/")
	if strings.HasSuffix(strings.ToLower(baseStr), "/emby") && strings.HasPrefix(strings.ToLower(path), "emby/") {
		path = path[len("emby/"):]
	}
	base, err := url.Parse(baseStr + "/")
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	target := base.ResolveReference(ref)
	target.RawQuery = rawQuery
	return target.String(), nil
}

func normalizeFnosURL(raw string, required bool) (string, error) {
	v := strings.TrimRight(strings.TrimSpace(raw), "/")
	if v == "" {
		if required {
			return "", fmt.Errorf("请填写飞牛影视地址")
		}
		return "", nil
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("飞牛影视地址格式不正确，示例：http://192.168.1.10:8005")
	}
	return v, nil
}

func normalizeOptionalPort(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("反代端口必须是 1-65535")
	}
	return strconv.Itoa(n), nil
}

func publicBase(r *http.Request, port string) string {
	if r == nil {
		if port == "" {
			return ""
		}
		return "http://127.0.0.1:" + port
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if port != "" {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = net.JoinHostPort(h, port)
		} else {
			host = net.JoinHostPort(strings.Split(host, ":")[0], port)
		}
	}
	return scheme + "://" + host
}

func extractItemID(fullPath string) string {
	parts := strings.Split(strings.Trim(fullPath, "/"), "/")
	for i, p := range parts {
		if strings.EqualFold(p, "Videos") || strings.EqualFold(p, "Items") || strings.EqualFold(p, "Audio") {
			if i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	return ""
}

func responseHeaders(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for k, values := range src {
		if _, skip := hopByHopHeaderNames[strings.ToLower(k)]; skip {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
	return dst
}

func copyRequestHeaders(dst, src http.Header, identity bool) {
	for k, values := range src {
		if _, skip := hopByHopHeaderNames[strings.ToLower(k)]; skip {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
	if identity {
		dst.Set("Accept-Encoding", "identity")
	}
}

func writeHeaders(w http.ResponseWriter, headers http.Header) {
	for k, values := range headers {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
}

func writeUpstreamBody(w http.ResponseWriter, resp *http.Response, body []byte) {
	writeHeaders(w, responseHeaders(resp.Header))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func rewriteLocation(location string, cfg Config, r *http.Request) string {
	fnosURL := strings.TrimRight(cfg.FnosURL, "/")
	if strings.HasPrefix(location, fnosURL) {
		return strings.TrimRight(publicBase(r, cfg.Port), "/") + strings.TrimPrefix(location, fnosURL)
	}
	return location
}

func queryValue(r *http.Request, key string) string {
	target := strings.ToLower(key)
	for k, values := range r.URL.Query() {
		if strings.ToLower(k) == target && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func stripMediaSourcePrefix(value string) string {
	return strings.TrimPrefix(value, "mediasource_")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func anyString(v any) string {
	switch got := v.(type) {
	case string:
		return got
	case json.Number:
		return got.String()
	case fmt.Stringer:
		return got.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}
}

func stringValue(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return ""
}

func mediaSources(m map[string]any) []map[string]any {
	if m == nil {
		return nil
	}
	raw, ok := m["MediaSources"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if ms, ok := item.(map[string]any); ok {
			out = append(out, ms)
		}
	}
	return out
}

var embyMediaStreamNonNullFields = []string{"Type", "Language", "DisplayLanguage", "Title", "DisplayTitle"}

// normalizeEmbyMediaStreams 补齐必填字符串字段
func normalizeEmbyMediaStreams(mediaSource map[string]any) bool {
	if mediaSource == nil {
		return false
	}
	raw, ok := mediaSource["MediaStreams"].([]any)
	if !ok {
		return false
	}
	changed := false
	for _, item := range raw {
		stream, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range embyMediaStreamNonNullFields {
			if v, exists := stream[field]; !exists || v == nil {
				stream[field] = ""
				changed = true
			}
		}
	}
	return changed
}
