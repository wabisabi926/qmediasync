package fnosproxy

import (
	"Q115-STRM/internal/helpers"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// handle 反代请求分发
func (s *Service) handle(w http.ResponseWriter, r *http.Request) {
	cfg := s.configFromDB()
	if !cfg.Enabled || cfg.FnosURL == "" {
		http.Error(w, "飞牛反代未启用", http.StatusNotFound)
		return
	}
	fullPath := strings.TrimPrefix(r.URL.Path, "/")
	if isStreamRequest(fullPath, r.URL.RawQuery) {
		s.redirectSTRMStream(w, r, cfg, fullPath)
		return
	}
	if itemFilePathRE.MatchString(fullPath) && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		s.redirectItemFile(w, r, cfg, fullPath)
		return
	}
	if playbackInfoPathRE.MatchString(fullPath) && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		s.modifyPlaybackInfo(w, r, cfg, fullPath)
		return
	}
	if baseHTMLPlayerPathRE.MatchString(fullPath) && r.Method == http.MethodGet {
		s.modifyBaseHTMLPlayer(w, r, cfg, fullPath)
		return
	}
	s.proxyRequest(w, r, cfg, fullPath)
}

func isStreamRequest(fullPath, rawQuery string) bool {
	pathLower := strings.ToLower("/" + strings.TrimPrefix(fullPath, "/"))
	for _, skip := range []string{"/images/", "/additionalparts", "/specialfeatures", "/subtitles"} {
		if strings.Contains(pathLower, skip) {
			return false
		}
	}
	if strings.Contains(pathLower, "/stream.") || strings.Contains(pathLower, "/original.") ||
		strings.Contains(pathLower, "/master.m3u8") {
		return true
	}
	if (strings.Contains(pathLower, "/stream") || strings.HasSuffix(pathLower, "/original") || strings.Contains(pathLower, "/original?")) &&
		(strings.Contains(pathLower, "/videos/") || strings.Contains(strings.ToLower(rawQuery), "mediasourceid=")) {
		return true
	}
	return false
}

// modifyPlaybackInfo 拦截 PlaybackInfo，缓存 STRM 媒体源并修改播放字段
func (s *Service) modifyPlaybackInfo(w http.ResponseWriter, r *http.Request, cfg Config, fullPath string) {
	upstreamPath := withEmbyAPIPrefix(fullPath)
	itemID := ""
	if m := playbackInfoPathRE.FindStringSubmatch(fullPath); len(m) > 1 {
		itemID = m[1]
	}
	resp, body, err := s.requestUpstreamWithRetry(r, cfg, upstreamPath, true)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	body = maybeGunzipBody(resp, body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeUpstreamBody(w, resp, body)
		return
	}
	if looksLikeHTML(body) {
		helpers.AppLogger.Warnf("飞牛反代 PlaybackInfo 收到 HTML，非 JSON: path=%s", upstreamPath)
		writeUpstreamBody(w, resp, body)
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeUpstreamBody(w, resp, body)
		return
	}
	if itemID == "" {
		itemID = strings.TrimSpace(anyString(payload["ItemId"]))
	}

	changed := false
	for _, mediaSource := range mediaSources(payload) {
		if normalizeEmbyMediaStreams(mediaSource) {
			changed = true
		}
		if s.rewriteStrmMediaSource(mediaSource, itemID, r, cfg) {
			changed = true
		}
	}
	if changed {
		if out, err := json.Marshal(payload); err == nil {
			body = out
		}
	}
	headers := responseHeaders(resp.Header)
	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	headers.Del("Content-Encoding")
	writeHeaders(w, headers)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// rewriteStrmMediaSource 缓存并预热 STRM 播放地址，改写播放能力字段
func (s *Service) rewriteStrmMediaSource(mediaSource map[string]any, itemID string, r *http.Request, cfg Config) bool {
	mediaSourceID := stringValue(mediaSource, "Id", "ID")
	rawPath := strings.TrimSpace(stringValue(mediaSource, "Path"))
	if !isStrmPath(rawPath) {
		return false
	}
	playURL := s.readStrmURL(rawPath, cfg)
	s.rememberSource(mediaSourceID, itemID, rawPath, playURL)

	id := firstNonEmpty(itemID, stripMediaSourcePrefix(mediaSourceID))
	mediaSource["SupportsDirectStream"] = true
	mediaSource["SupportsTranscoding"] = false
	mediaSource["DirectStreamUrl"] = proxiedVideoPath(r, id, mediaSourceID)
	mediaSource["Protocol"] = "Http"
	mediaSource["IsRemote"] = true
	mediaSource["SupportsDirectPlay"] = true
	delete(mediaSource, "TranscodingUrl")
	delete(mediaSource, "TranscodingSubProtocol")
	delete(mediaSource, "TranscodingContainer")
	return true
}

func proxiedVideoPath(r *http.Request, itemID, mediaSourceID string) string {
	q := r.URL.Query()
	q.Set("MediaSourceId", mediaSourceID)
	if q.Get("static") == "" {
		q.Set("static", "true")
	}
	return "/Videos/" + itemID + "/stream?" + q.Encode()
}

// modifyBaseHTMLPlayer 修改播放器JS，移除 crossorigin 限制
func (s *Service) modifyBaseHTMLPlayer(w http.ResponseWriter, r *http.Request, cfg Config, fullPath string) {
	resp, body, err := s.requestUpstream(r, cfg, fullPath, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	crossOriginGuard := []byte(";try{(function(){var s=Element.prototype.setAttribute;Element.prototype.setAttribute=function(n,v){if(this&&this.tagName&&/^(VIDEO|AUDIO)$/i.test(this.tagName)&&String(n).toLowerCase()==='crossorigin')return;return s.call(this,n,v)};try{Object.defineProperty(HTMLMediaElement.prototype,'crossOrigin',{get:function(){return null},set:function(){return null},configurable:true})}catch(e){}})()}catch(e){};")
	body = bytes.ReplaceAll(body, []byte(`mediaSource.IsRemote&&"DirectPlay"===playMethod?null:"anonymous"`), []byte("null"))
	body = htmlCrossOriginRE.ReplaceAll(body, []byte("null"))
	if !bytes.Contains(body, []byte("HTMLMediaElement.prototype,'crossOrigin'")) {
		body = append(crossOriginGuard, body...)
	}
	headers := responseHeaders(resp.Header)
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	headers.Set("Cache-Control", "no-store")
	headers.Del("Content-Encoding")
	writeHeaders(w, headers)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// proxyRequest 直接反向代理到飞牛影视
func (s *Service) proxyRequest(w http.ResponseWriter, r *http.Request, cfg Config, fullPath string) {
	targetValue, err := targetURL(cfg, fullPath, r.URL.RawQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	target, err := url.Parse(targetValue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	proxy := &httputil.ReverseProxy{
		Transport: s.client.Transport,
		Rewrite: func(req *httputil.ProxyRequest) {
			outURL := *target
			req.Out.URL = &outURL
			req.Out.Host = target.Host
			req.SetXForwarded()
		},
		ModifyResponse: func(resp *http.Response) error {
			if loc := resp.Header.Get("Location"); loc != "" {
				resp.Header.Set("Location", rewriteLocation(loc, cfg, r))
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			if req.Context().Err() != nil {
				return
			}
			helpers.AppLogger.Warnf("飞牛反代请求失败: path=%s, error=%v", req.URL.Path, err)
			http.Error(w, "飞牛影视服务暂时无法访问", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
}

func (s *Service) requestUpstreamWithRetry(r *http.Request, cfg Config, fullPath string, identity bool) (*http.Response, []byte, error) {
	var lastErr error
	for attempt := 1; attempt <= playbackInfoRetries; attempt++ {
		resp, body, err := s.requestUpstream(r, cfg, fullPath, identity)
		if err != nil {
			lastErr = err
			if attempt < playbackInfoRetries {
				time.Sleep(playbackInfoRetryGap)
			}
			continue
		}
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			if attempt < playbackInfoRetries {
				time.Sleep(playbackInfoRetryGap)
				continue
			}
			resp.Body = io.NopCloser(bytes.NewReader(body))
		}
		return resp, body, nil
	}
	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, errors.New("PlaybackInfo 重试失败")
}

func (s *Service) requestUpstream(r *http.Request, cfg Config, fullPath string, identity bool) (*http.Response, []byte, error) {
	target, err := targetURL(cfg, fullPath, r.URL.RawQuery)
	if err != nil {
		return nil, nil, err
	}
	var body io.Reader
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		buf, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(buf))
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		return nil, nil, err
	}
	copyRequestHeaders(req.Header, r.Header, identity)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return resp, data, nil
}

// withEmbyAPIPrefix 保证飞牛 API 走 /emby 前缀
func withEmbyAPIPrefix(fullPath string) string {
	p := strings.TrimPrefix(strings.TrimSpace(fullPath), "/")
	if p == "" {
		return p
	}
	if strings.HasPrefix(strings.ToLower(p), "emby/") {
		return p
	}
	return "emby/" + p
}

func maybeGunzipBody(resp *http.Response, body []byte) []byte {
	if resp == nil || !strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		return body
	}
	gr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return body
	}
	defer gr.Close()
	out, err := io.ReadAll(gr)
	if err != nil {
		return body
	}
	return out
}

func looksLikeHTML(body []byte) bool {
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 {
		return false
	}
	lower := bytes.ToLower(trim)
	return bytes.HasPrefix(lower, []byte("<!doctype html")) ||
		bytes.HasPrefix(lower, []byte("<html"))
}
