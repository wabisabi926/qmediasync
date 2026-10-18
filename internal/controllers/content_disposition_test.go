package controllers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 测试 302 响应 Content-Disposition 文件名编码（RFC 5987）
func TestSetRedirectContentDisposition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name     string
		fileName string
		wantSub  string // 期望包含的片段
		wantNone string // 期望不包含的片段
	}{
		{
			name:     "中文文件名",
			fileName: "流浪地球2.iso",
			wantSub:  `filename*=UTF-8''%E6%B5%81%E6%B5%AA%E5%9C%B0%E7%90%832.iso`,
		},
		{
			name:     "空格与井号",
			fileName: "Movie 2024 #1.mkv",
			wantSub:  `filename*=UTF-8''Movie%202024%20%231.mkv`,
		},
		{
			name:     "纯ASCII文件名",
			fileName: "movie.mkv",
			wantSub:  `filename="movie.mkv"; filename*=UTF-8''movie.mkv`,
		},
		{
			name:     "携带网盘完整路径取最后一段",
			fileName: "/电影/流浪地球/流浪地球2.iso",
			wantSub:  `filename*=UTF-8''%E6%B5%81%E6%B5%AA%E5%9C%B0%E7%90%832.iso`,
			wantNone: "%2F",
		},
		{
			name:     "空文件名不设置头",
			fileName: "",
			wantSub:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			setRedirectContentDisposition(c, tc.fileName)
			got := w.Header().Get("Content-Disposition")
			if tc.wantSub == "" {
				if got != "" {
					t.Fatalf("期望不设置头, 实际: %q", got)
				}
				return
			}
			if got == "" {
				t.Fatalf("期望设置 Content-Disposition, 实际为空")
			}
			if tc.wantNone != "" && strings.Contains(got, tc.wantNone) {
				t.Fatalf("不应包含 %q, 实际: %q", tc.wantNone, got)
			}
			if !strings.Contains(got, tc.wantSub) {
				t.Fatalf("期望包含 %q, 实际: %q", tc.wantSub, got)
			}
		})
	}
}
