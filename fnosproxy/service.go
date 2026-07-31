package fnosproxy

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	sourceCacheMaxEntries = 256
	sourceCacheTTL        = 24 * time.Hour
	testRequestTimeout    = 20 * time.Second
	playbackInfoRetries   = 2
	playbackInfoRetryGap  = 500 * time.Millisecond
)

var (
	videoStreamPathRE    = regexp.MustCompile(`(?i)^(?:/?emby)?/?Videos/([^/]+)/(stream|original)(?:\.\w+)?$`)
	playbackInfoPathRE   = regexp.MustCompile(`(?i)^(?:/?emby)?/?Items/([^/]+)/PlaybackInfo$`)
	itemFilePathRE       = regexp.MustCompile(`(?i)^(?:/?emby)?/?Items/([^/]+)/(Download|File)$`)
	baseHTMLPlayerPathRE = regexp.MustCompile(`(?i)^(?:/?emby)?/?web/modules/htmlvideoplayer/basehtmlplayer\.js$`)
	htmlCrossOriginRE    = regexp.MustCompile(`mediaSource\.IsRemote\s*&&\s*(?:"DirectPlay"\s*===\s*playMethod|playMethod\s*===\s*"DirectPlay")\s*\?\s*null\s*:\s*"anonymous"`)
	hopByHopHeaderNames  = map[string]struct{}{"connection": {}, "keep-alive": {}, "proxy-authenticate": {}, "proxy-authorization": {}, "te": {}, "trailers": {}, "transfer-encoding": {}, "upgrade": {}, "host": {}}
)

// envType 返回当前运行环境类型
func envType() string {
	if runtime.GOOS == "windows" {
		return "windows"
	}
	if helpers.IsFnOS {
		return "fnos"
	}
	return "docker"
}

type cachedSource struct {
	MediaSourceID string
	ItemID        string
	Path          string
	URL           string
	LastUsed      time.Time
}

// Service 飞牛影视反代服务
type Service struct {
	client *http.Client

	mu     sync.Mutex
	server *http.Server
	port   string
	err    string

	cacheMu                sync.Mutex
	byMS                   map[string]*cachedSource
	byItem                 map[string]*cachedSource
	cacheEntries           map[*cachedSource]struct{}
	cacheConfig            string
	cacheConfigInitialized bool
}

var globalService *Service

// GetService 获取全局反代服务实例
func GetService() *Service {
	if globalService == nil {
		globalService = newService()
	}
	return globalService
}

func newService() *Service {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DisableCompression: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Service{
		client:       client,
		byMS:         map[string]*cachedSource{},
		byItem:       map[string]*cachedSource{},
		cacheEntries: map[*cachedSource]struct{}{},
	}
}

// Config 反代配置快照
type Config struct {
	Enabled   bool   `json:"enabled"`
	FnosURL   string `json:"fnos_url"`
	Port      string `json:"port"`
	PathMaps  string `json:"path_maps"`
	ProxyURL  string `json:"proxy_url"`
	Running   bool   `json:"running"`
	LastError string `json:"last_error,omitempty"`
	Env       string `json:"env"` // windows / fnos / docker
}

// UpdateRequest 更新请求
type UpdateRequest struct {
	Enabled  bool   `json:"enabled"`
	FnosURL  string `json:"fnos_url"`
	Port     string `json:"port"`
	PathMaps string `json:"path_maps"`
}

// Snapshot 获取当前配置快照
func (s *Service) Snapshot(r *http.Request) Config {
	cfg := s.configFromDB()
	if cfg.Port != "" {
		cfg.ProxyURL = publicBase(r, cfg.Port)
	}
	cfg.Env = envType()
	s.mu.Lock()
	cfg.Running = s.server != nil
	cfg.LastError = s.err
	s.mu.Unlock()
	return cfg
}

// Update 更新配置并重启服务
func (s *Service) Update(ctx context.Context, in UpdateRequest) (Config, error) {
	fnosURL, err := normalizeFnosURL(in.FnosURL, false)
	if err != nil {
		return Config{}, err
	}
	port, err := normalizeOptionalPort(in.Port)
	if err != nil {
		return Config{}, err
	}
	pathMaps := normalizePathMaps(in.PathMaps)
	if in.Enabled && port != "" {
		if fnosURL == "" {
			return Config{}, fmt.Errorf("启用反代并填写端口时，需要填写飞牛影视地址")
		}
	}
	// 保存到数据库
	config, err := models.GetFnosProxyConfig()
	if err != nil {
		return Config{}, err
	}
	enabledVal := 0
	if in.Enabled {
		enabledVal = 1
	}
	if err := config.Update(map[string]interface{}{
		"enabled":   enabledVal,
		"fnos_url":  fnosURL,
		"port":      port,
		"path_maps": pathMaps,
	}); err != nil {
		return Config{}, err
	}
	if err := s.Sync(ctx); err != nil {
		return s.Snapshot(nil), err
	}
	return s.Snapshot(nil), nil
}

// Test 测试飞牛影视地址连通性
func (s *Service) Test(ctx context.Context) error {
	return s.TestConfig(ctx, s.configFromDB())
}

// TestUpdate 用更新请求测试连通性
func (s *Service) TestUpdate(ctx context.Context, in UpdateRequest) error {
	fnosURL, err := normalizeFnosURL(in.FnosURL, false)
	if err != nil {
		return err
	}
	return s.TestConfig(ctx, Config{FnosURL: fnosURL})
}

// TestConfig 测试配置中的飞牛地址
func (s *Service) TestConfig(ctx context.Context, cfg Config) error {
	if strings.TrimSpace(cfg.FnosURL) == "" {
		return fmt.Errorf("请先填写飞牛影视地址")
	}
	testCtx, cancel := context.WithTimeout(ctx, testRequestTimeout)
	defer cancel()
	candidates := []string{
		cfg.FnosURL + "/System/Info/Public",
		cfg.FnosURL + "/System/Info",
		cfg.FnosURL + "/",
	}
	var lastErr error
	for _, testURL := range candidates {
		req, err := http.NewRequestWithContext(testCtx, http.MethodGet, testURL, nil)
		if err != nil {
			return fmt.Errorf("飞牛影视地址无效")
		}
		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()
		if resp.StatusCode < 500 {
			return nil
		}
		lastErr = fmt.Errorf("飞牛影视返回状态码 %d", resp.StatusCode)
	}
	if lastErr != nil {
		return fmt.Errorf("飞牛影视地址无法访问: %v", lastErr)
	}
	return fmt.Errorf("飞牛影视地址无法访问")
}

// Start 启动反代服务
func (s *Service) Start(ctx context.Context) {
	if err := s.Sync(ctx); err != nil {
		helpers.AppLogger.Warnf("飞牛反代启动失败: %v", err)
	}
}

// Sync 同步配置，按需启动/停止服务
func (s *Service) Sync(ctx context.Context) error {
	cfg := s.configFromDB()
	s.syncSourceCacheConfig(cfg)

	s.mu.Lock()
	defer s.mu.Unlock()

	// 配置变更或禁用时停止旧服务
	if s.server != nil && (cfg.Port == "" || !cfg.Enabled || s.port != cfg.Port) {
		s.stopLocked(ctx)
	}
	s.err = ""
	if !cfg.Enabled || cfg.Port == "" {
		return nil
	}
	if cfg.FnosURL == "" {
		s.err = "启用反代时需要填写飞牛影视地址"
		return fmt.Errorf("%s", s.err)
	}
	if s.server != nil && s.port == cfg.Port {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		s.err = fmt.Sprintf("飞牛反代端口 %s 监听失败：%v", cfg.Port, err)
		return fmt.Errorf("%s", s.err)
	}
	s.server = srv
	s.port = cfg.Port
	go func() {
		helpers.AppLogger.Infof("飞牛反代已监听 %s", srv.Addr)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.err = err.Error()
			s.server = nil
			s.port = ""
			s.mu.Unlock()
			helpers.AppLogger.Errorf("飞牛反代服务异常退出: %v", err)
		}
	}()
	return nil
}

// Shutdown 关闭反代服务
func (s *Service) Shutdown(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked(ctx)
}

func (s *Service) stopLocked(ctx context.Context) {
	if s.server == nil {
		return
	}
	srv := s.server
	s.server = nil
	s.port = ""
	stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(stopCtx); err != nil {
		_ = srv.Close()
	}
}

func (s *Service) configFromDB() Config {
	config, err := models.GetFnosProxyConfig()
	if err != nil {
		return Config{}
	}
	return Config{
		Enabled:  config.Enabled == 1,
		FnosURL:  strings.TrimRight(strings.TrimSpace(config.FnosURL), "/"),
		Port:     strings.TrimSpace(config.Port),
		PathMaps: strings.TrimSpace(config.PathMaps),
	}
}
