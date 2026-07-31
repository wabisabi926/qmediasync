package fnosproxy

import (
	"net/url"
	"os"
	"strings"
)

// === STRM 文件读取 ===

// readStrmURL 读取 STRM 文件内容，返回其中的 HTTP 直链
func (s *Service) readStrmURL(rawPath string, cfg Config) string {
	candidates := strmPathCandidates(rawPath, s.resolvePathMaps(cfg.PathMaps))
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
				return line
			}
			break
		}
	}
	return ""
}

func (s *Service) resolvePathMaps(raw string) [][2]string {
	roots := parsePathMaps(raw)
	out := make([][2]string, 0, len(roots))
	for _, m := range roots {
		out = append(out, m)
	}
	// 按飞牛路径长度降序排列，优先匹配更长的路径
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if len(out[j][0]) > len(out[i][0]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// parsePathMaps 解析路径映射，格式：每行 "飞牛路径|本地路径"
func parsePathMaps(raw string) [][2]string {
	var out [][2]string
	seen := map[string]struct{}{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(line), "\\", "/"), "/")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		from := strings.TrimRight(strings.TrimSpace(parts[0]), "/")
		to := strings.TrimRight(strings.TrimSpace(parts[1]), "/")
		if from == "" || to == "" {
			continue
		}
		if _, ok := seen[from]; ok {
			continue
		}
		seen[from] = struct{}{}
		out = append(out, [2]string{from, to})
	}
	return out
}

func normalizePathMaps(raw string) string {
	maps := parsePathMaps(raw)
	var lines []string
	for _, m := range maps {
		lines = append(lines, m[0]+"|"+m[1])
	}
	return strings.Join(lines, "\n")
}

func strmPathCandidates(rawPath string, maps [][2]string) []string {
	rawPath = strings.TrimSpace(strings.ReplaceAll(rawPath, "\\", "/"))
	if strings.HasPrefix(rawPath, "file://") {
		if u, err := url.Parse(rawPath); err == nil {
			rawPath = u.Path
		}
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	add(rawPath)
	for _, m := range maps {
		if strings.HasPrefix(rawPath, m[0]) {
			add(m[1] + strings.TrimPrefix(rawPath, m[0]))
		}
	}
	return out
}

func isStrmPath(value string) bool {
	v := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")))
	if strings.HasPrefix(v, "file://") {
		if u, err := url.Parse(v); err == nil {
			v = strings.ToLower(u.Path)
		}
	}
	return strings.HasSuffix(v, ".strm")
}
