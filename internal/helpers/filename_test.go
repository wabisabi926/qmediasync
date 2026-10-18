package helpers

import (
	"runtime"
	"testing"
)

func TestSanitizeWindowsSegment(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"冒号换全角", "电影: 加长版", "电影： 加长版"},
		{"问号换下划线", "第1集?", "第1集_"},
		{"星号换下划线", "第1集*", "第1集_"},
		{"竖线换下划线", "A|B", "A_B"},
		{"尖括号换下划线", "<A>", "_A_"},
		{"双引号换下划线", `A"B`, "A_B"},
		{"反斜杠换下划线", `A\B`, "A_B"},
		{"正斜杠换下划线", "A/B", "A_B"},
		{"去掉结尾空格", "电影 ", "电影"},
		{"去掉结尾点", "电影.", "电影"},
		{"点目录保留", ".", "."},
		{"父目录保留", "..", ".."},
		{"空串兜底", "", "_"},
		{"正常中文不变", "流浪地球2", "流浪地球2"},
		{"已是全角冒号不变", "电影：加长版", "电影：加长版"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeWindowsSegment(tc.in); got != tc.want {
				t.Fatalf("sanitizeWindowsSegment(%q) = %q, 期望 %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeRelPathKeepsSeparators(t *testing.T) {
	in := "/电影: 合集/第1集?.mkv"
	want := "/电影： 合集/第1集_.mkv"
	if runtime.GOOS != "windows" {
		want = in
	}
	if got := SanitizeRelPath(in); got != want {
		t.Fatalf("SanitizeRelPath(%q) = %q, 期望 %q", in, got, want)
	}
}

func TestSanitizeFileNameLeavesOtherPlatformAlone(t *testing.T) {
	in := "电影: 第1集?.strm"
	want := "电影： 第1集_.strm"
	if runtime.GOOS != "windows" {
		want = in
	}
	if got := SanitizeFileName(in); got != want {
		t.Fatalf("SanitizeFileName(%q) = %q, 期望 %q", in, got, want)
	}
}
