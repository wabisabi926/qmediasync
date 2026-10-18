package helpers

import (
	"path/filepath"
	"runtime"
	"strings"
)

// windowsSanitizeReplacer 把 Windows 文件名非法字符替换为下划线
// 冒号单独处理为全角，尽量保留片名可读性
var windowsSanitizeReplacer = strings.NewReplacer(
	`<`, "_",
	`>`, "_",
	`"`, "_",
	`/`, "_",
	`\`, "_",
	`|`, "_",
	`?`, "_",
	`*`, "_",
)

// StrmFileName 把视频文件名转换为 strm 文件名。
// isoFilename 为 true 时 ISO 原盘生成 xxx.iso.strm，便于 Emby/Infuse 按 ISO 容器识别；
// 其余情况统一为 xxx.strm。
func StrmFileName(name string, isoFilename bool) string {
	ext := filepath.Ext(name)
	baseName := strings.TrimSuffix(name, ext)
	if isoFilename && strings.EqualFold(ext, ".iso") {
		return baseName + ".iso.strm"
	}
	return baseName + ".strm"
}

// MaxPathComponentBytes 文件系统单个路径分量的最大字节数（ext4/NTFS 均为 255）
const MaxPathComponentBytes = 255

// OversizedPathComponent 返回路径中第一个超过长度限制的分量，没有超长分量时返回空串。
// 超长路径无法落盘，调用方应跳过并记录明确原因，而不是让写入静默失败。
func OversizedPathComponent(fullPath string) string {
	if fullPath == "" {
		return ""
	}
	for _, part := range strings.Split(filepath.ToSlash(fullPath), "/") {
		if part == "" {
			continue
		}
		if len(part) > MaxPathComponentBytes {
			return part
		}
	}
	return ""
}

// SanitizeFileName 清理文件名中的非法字符，避免在 Windows 上创建文件失败。
// 非 Windows 平台直接返回原值：这些字符在 Linux/macOS 上合法，
// 强行替换会让本地文件名与网盘原名不一致，进而导致路径匹配错位。
func SanitizeFileName(name string) string {
	if runtime.GOOS != "windows" || name == "" {
		return name
	}
	return sanitizeWindowsSegment(name)
}

// SanitizeRelPath 逐段清理相对路径中的非法字符，保留 / 作为分隔符。
// 用于把网盘路径转换为本地路径：目录名里的非法字符同样会导致整条路径写入失败。
func SanitizeRelPath(relPath string) string {
	if runtime.GOOS != "windows" || relPath == "" {
		return relPath
	}
	segments := strings.Split(strings.ReplaceAll(relPath, `\`, "/"), "/")
	for i, seg := range segments {
		// 空段来自路径开头或连续的 /，保持原样，避免破坏路径结构
		if seg == "" {
			continue
		}
		segments[i] = sanitizeWindowsSegment(seg)
	}
	return strings.Join(segments, "/")
}

// sanitizeWindowsSegment 清理单个路径段
func sanitizeWindowsSegment(seg string) string {
	// . 和 .. 是路径语义，不能按文件名规则裁剪
	if seg == "." || seg == ".." {
		return seg
	}
	seg = strings.ReplaceAll(seg, ":", "：")
	seg = windowsSanitizeReplacer.Replace(seg)
	// Windows 不允许文件名以空格或点结尾
	seg = strings.TrimRight(seg, " .")
	if seg == "" {
		return "_"
	}
	return seg
}
