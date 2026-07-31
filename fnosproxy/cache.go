package fnosproxy

import (
	"strings"
	"time"
)

// === 源缓存 ===

func (s *Service) rememberSource(mediaSourceID, itemID, strmPath, playURL string) {
	now := time.Now()
	mediaSourceID = strings.TrimSpace(mediaSourceID)
	itemID = strings.TrimSpace(itemID)
	key := stripMediaSourcePrefix(mediaSourceID)
	if mediaSourceID == "" && itemID == "" {
		return
	}
	src := &cachedSource{
		MediaSourceID: mediaSourceID,
		ItemID:        itemID,
		Path:          strings.TrimSpace(strmPath),
		URL:           strings.TrimSpace(playURL),
		LastUsed:      now,
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.pruneExpiredSourceCacheLocked(now)
	if mediaSourceID != "" {
		s.removeSourceLocked(s.byMS[mediaSourceID])
	}
	if key != "" {
		s.removeSourceLocked(s.byMS[key])
	}
	if itemID != "" {
		s.removeSourceLocked(s.byItem[itemID])
	}
	s.cacheEntries[src] = struct{}{}
	if key != "" {
		s.byMS[key] = src
	}
	if mediaSourceID != "" {
		s.byMS[mediaSourceID] = src
	}
	if src.ItemID != "" {
		s.byItem[src.ItemID] = src
	}
	s.enforceSourceCacheLimitLocked()
}

func (s *Service) lookupCached(mediaSourceID, itemID string) *cachedSource {
	now := time.Now()
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.pruneExpiredSourceCacheLocked(now)
	var src *cachedSource
	if mediaSourceID != "" {
		src = s.byMS[mediaSourceID]
		if src == nil {
			src = s.byMS[stripMediaSourcePrefix(mediaSourceID)]
		}
	}
	if src == nil && itemID != "" {
		src = s.byItem[itemID]
	}
	if src == nil {
		return nil
	}
	src.LastUsed = now
	snapshot := *src
	return &snapshot
}

func (s *Service) syncSourceCacheConfig(cfg Config) {
	signature := strings.TrimRight(strings.TrimSpace(cfg.FnosURL), "/") + "\x00" + normalizePathMaps(cfg.PathMaps)
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if !s.cacheConfigInitialized {
		s.cacheConfig = signature
		s.cacheConfigInitialized = true
		return
	}
	if s.cacheConfig == signature {
		return
	}
	s.clearSourceCacheLocked()
	s.cacheConfig = signature
}

func (s *Service) clearSourceCacheLocked() {
	clear(s.byMS)
	clear(s.byItem)
	clear(s.cacheEntries)
}

func (s *Service) pruneExpiredSourceCacheLocked(now time.Time) {
	for src := range s.cacheEntries {
		if !src.LastUsed.Add(sourceCacheTTL).After(now) {
			s.removeSourceLocked(src)
		}
	}
}

func (s *Service) enforceSourceCacheLimitLocked() {
	for len(s.cacheEntries) > sourceCacheMaxEntries {
		var oldest *cachedSource
		for src := range s.cacheEntries {
			if oldest == nil || src.LastUsed.Before(oldest.LastUsed) {
				oldest = src
			}
		}
		if oldest == nil {
			return
		}
		s.removeSourceLocked(oldest)
	}
}

func (s *Service) removeSourceLocked(src *cachedSource) {
	if src == nil {
		return
	}
	if src.MediaSourceID != "" {
		if s.byMS[src.MediaSourceID] == src {
			delete(s.byMS, src.MediaSourceID)
		}
		key := stripMediaSourcePrefix(src.MediaSourceID)
		if key != "" && s.byMS[key] == src {
			delete(s.byMS, key)
		}
	}
	if src.ItemID != "" && s.byItem[src.ItemID] == src {
		delete(s.byItem, src.ItemID)
	}
	delete(s.cacheEntries, src)
}
