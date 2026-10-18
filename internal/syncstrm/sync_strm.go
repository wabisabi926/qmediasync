package syncstrm

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

type StrmData struct {
	UserId   string `json:"userid"`    // 用户ID
	PickCode string `json:"pick_code"` // 文件ID
	Sign     string `json:"sign"`      // 文件签名
	Path     string `json:"path"`      // 115的路径
	BaseUrl  string `json:"base_url"`  // 115的base_url
	UrlPath  string `json:"url_path"`  // 115的url_path
}

// 生成strm文件
// st只能是来源路径，所以需要生成strm文件的路径
func (s *SyncStrm) ProcessStrmFile(sf *SyncFileCache) error {
	// 开关切换后，把另一种命名规则的旧 ISO strm 文件迁移掉，避免 Emby 里出现重复条目
	s.migrateLegacyISOFilename(sf)
	rs := s.CompareStrm(sf)
	if rs == 1 {
		// s.Sync.Logger.Infof("文件 %s 已存在且无需更新strm文件，跳过", filepath.Join(sf.Path, sf.FileName))
		return nil
	}
	// localFilePath := sf.GetLocalFilePath()
	strmFullPath := sf.GetLocalFilePath(s.TargetPath, s.SourcePath)
	// 超长路径无法落盘，直接跳过并记录明确原因
	if oversized := helpers.OversizedPathComponent(strmFullPath); oversized != "" {
		s.Sync.Logger.Warnf("strm 路径超长已跳过（单段最多 %d 字节）: %s", helpers.MaxPathComponentBytes, strmFullPath)
		return nil
	}
	strmContent := s.SyncDriver.MakeStrmContent(sf)
	if strmContent == "" {
		s.Sync.Logger.Errorf("生成strm文件内容失败，可能是STRM直连地址格式不正确: %s", filepath.Join(sf.Path, sf.FileName))
		return fmt.Errorf("生成strm文件内容失败")
	}

	// 写入文件并设置所有者
	err := helpers.WriteFileWithPerm(strmFullPath, []byte(strmContent), 0777)
	if err != nil {
		s.Sync.Logger.Errorf("写入strm文件并设置所有者失败: %v", err)
		return err
	}
	// 修改文件时间
	if sf.MTime > 0 {
		err := os.Chtimes(strmFullPath, time.Unix(sf.MTime, 0), time.Unix(sf.MTime, 0))
		if err != nil {
			s.Sync.Logger.Errorf("修改strm文件时间失败: %v", err)
			return err
		}
	}
	s.Sync.Logger.Infof("[生成strm] %s => %s", strmFullPath, strmContent)
	atomic.AddInt64(&s.NewStrm, 1)
	return nil
}

// 1-无需操作，0-更新
func (s *SyncStrm) CompareStrm(st *SyncFileCache) int {
	localFilePath := st.GetLocalFilePath(s.TargetPath, s.SourcePath)
	if !helpers.PathExists(localFilePath) {
		// s.Sync.Logger.Infof("文件 %s 不存在，需要生成strm文件", st.LocalFilePath)
		return 0
	}
	if st.SourceType == models.SourceTypeLocal {
		// s.Sync.Logger.Infof("文件 %s 来源本地，不需要生成strm文件", filepath.Join(st.Path, st.FileName))
		return 1
	}
	// 读取strm文件内容
	strmData := s.LoadDataFromStrm(localFilePath)
	if strmData == nil {
		return 0
	}
	if st.SourceType == models.SourceTypeOpenList {
		account, err := models.GetAccountById(s.Account.ID)
		if err != nil {
			s.Sync.Logger.Errorf("获取Openlist账号信息失败: %v", err)
			return 0
		}
		baseUrl := s.Config.StrmBaseUrl
		if baseUrl == "" {
			baseUrl = account.BaseUrl
		}
		// 如果baseUrl以/结尾，删掉结尾的/
		if before, ok := strings.CutSuffix(baseUrl, "/"); ok {
			baseUrl = before
		}
		// 比较主机名称是否相同
		if strmData.BaseUrl != baseUrl {
			s.Sync.Logger.Warnf("文件 %s 的STRM内容的主机名称与本地不一致, 本地: %s, 远程: %s", filepath.Join(st.Path, st.FileName), baseUrl, strmData.BaseUrl)
			return 0
		}
		if strmData.Sign != st.OpenlistSign {
			s.Sync.Logger.Warnf("文件 %s 的STRM内容的签名参数与本地不一致, 本地: %s, 远程: %s", filepath.Join(st.Path, st.FileName), st.OpenlistSign, strmData.Sign)
			return 0
		}
	}
	if st.SourceType == models.SourceType115 || st.SourceType == models.SourceTypeBaiduPan {
		// 比较路径是否相同
		if s.Config.StrmUrlNeedPath == 1 {
			stPath := filepath.ToSlash(filepath.Join(st.Path, st.FileName))
			if strmData.Path != stPath {
				s.Sync.Logger.Warnf("文件 %s 的STRM内容的路径与本地不一致, 本地: %s, 远程: %s", stPath, stPath, strmData.Path)
				return 0
			}
		} else {
			if strmData.Path != "" {
				s.Sync.Logger.Warnf("文件 %s 的STRM内容的含有完整路径 %s，但是设置中关闭了添加路径，所以重新生成strm以去掉路径s", filepath.Join(st.Path, st.FileName), strmData.Path)
				return 0
			}
		}
		// 比较主机名称是否相同
		// 如果StrmBaseUrl以/结尾，那删除掉末尾的/
		if before, ok := strings.CutSuffix(s.Config.StrmBaseUrl, "/"); ok {
			s.Config.StrmBaseUrl = before
		}
		if strmData.BaseUrl != s.Config.StrmBaseUrl {
			s.Sync.Logger.Warnf("文件 %s 的STRM内容的主机名称与本地不一致, 本地: %s, 远程: %s", filepath.Join(st.Path, st.FileName), s.Config.StrmBaseUrl, strmData.BaseUrl)
			return 0
		}
		// 如果没有PickCode，则更新以补全
		if strmData.PickCode == "" {
			s.Sync.Logger.Warnf("文件 %s 的STRM内容缺少PickCode: %s, 补全", filepath.Join(st.Path, st.FileName), strmData.PickCode)
			return 0
		} else {
			if strmData.PickCode != st.PickCode {
				s.Sync.Logger.Warnf("文件 %s 的STRM内容的PickCode与本地不一致, 本地: %s, 远程: %s", filepath.Join(st.Path, st.FileName), st.PickCode, strmData.PickCode)
				return 0
			}
		}
		if strmData.UserId != s.Account.UserId {
			s.Sync.Logger.Warnf("文件 %s 的STRM内容的用户ID与本地不一致, 本地: %s, 远程: %s", filepath.Join(st.Path, st.FileName), s.Account.UserId, strmData.UserId)
			return 0
		}
		// 比较UrlPath，如果是iso文件，UrlPath必须以.iso结尾
		ext := filepath.Ext(st.FileName)
		if !strings.HasSuffix(strmData.UrlPath, ext) {
			s.Sync.Logger.Warnf("文件 %s 的STRM内容的Url路径 %s 没有以 %s 结尾，重新生成", filepath.Join(st.Path, st.FileName), strmData.UrlPath, ext)
			return 0
		}
	}
	return 1
}

// migrateLegacyISOFilename 在 ISO 文件名兼容开关切换后，迁移旧命名规则的 strm 文件。
// 开启开关时把 xxx.strm 迁移为 xxx.iso.strm；关闭时反向迁移。
// 避免同一个 ISO 在 Emby 里出现两个条目。
func (s *SyncStrm) migrateLegacyISOFilename(sf *SyncFileCache) {
	if !sf.IsVideo || !strings.EqualFold(filepath.Ext(sf.FileName), ".iso") {
		return
	}
	enabled := models.SettingsGlobal.ISOFilename == 1
	currentPath := sf.GetLocalFilePath(s.TargetPath, s.SourcePath)
	// 另一种命名规则下的旧路径
	legacyPath := sf.LocalFilePathFor(s.TargetPath, s.SourcePath, !enabled)
	if legacyPath == "" || currentPath == "" || legacyPath == currentPath {
		return
	}
	if !helpers.PathExists(legacyPath) {
		return
	}
	// 新文件已存在时，直接删掉旧文件
	if helpers.PathExists(currentPath) {
		if err := os.Remove(legacyPath); err != nil {
			s.Sync.Logger.Warnf("删除旧 ISO strm 文件失败: %s, %v", legacyPath, err)
		} else {
			s.Sync.Logger.Infof("已删除旧 ISO strm 文件: %s", legacyPath)
		}
		return
	}
	// 重命名迁移
	if err := os.Rename(legacyPath, currentPath); err != nil {
		s.Sync.Logger.Warnf("迁移旧 ISO strm 文件失败: %s => %s, %v", legacyPath, currentPath, err)
		return
	}
	s.Sync.Logger.Infof("已迁移旧 ISO strm 文件: %s => %s", legacyPath, currentPath)
}
func (s *SyncStrm) LoadDataFromStrm(strmPath string) *StrmData {
	if !helpers.PathExists(strmPath) {
		// s.Sync.Logger.Errorf("strm文件不存在: %s", strmPath)
		return nil
	}
	data, err := os.ReadFile(strmPath)
	if err != nil {
		s.Sync.Logger.Errorf("读取strm文件失败: %v", err)
		return nil
	}
	var strmData StrmData
	strmUrl, urlErr := url.Parse(string(data))
	if urlErr != nil {
		s.Sync.Logger.Errorf("解析strm文件失败: %v", urlErr)
		return nil
	}
	strmData.UrlPath = strmUrl.Path
	queryParams := strmUrl.Query()
	if pickCode := queryParams.Get("pickcode"); pickCode != "" {
		strmData.PickCode = pickCode
	}
	if userId := queryParams.Get("userid"); userId != "" {
		strmData.UserId = userId
	}
	strmData.Sign = ""
	if sign := queryParams.Get("sign"); sign != "" {
		strmData.Sign = sign
	}
	strmData.Path = ""
	if path := queryParams.Get("path"); path != "" {
		strmData.Path = path
	}
	strmData.BaseUrl = fmt.Sprintf("%s://%s", strmUrl.Scheme, strmUrl.Host)
	return &strmData
}

func (sf *SyncStrm) GetRemoteFilePathUrlEncode(path string) string {
	// 中文保留，只对特殊字符编码
	path = strings.ReplaceAll(path, "%", "%25")
	path = strings.ReplaceAll(path, "/", "%2F")
	path = strings.ReplaceAll(path, "?", "%3F")
	path = strings.ReplaceAll(path, "&", "%26")
	path = strings.ReplaceAll(path, "=", "%3D")
	path = strings.ReplaceAll(path, "+", "%2B")
	path = strings.ReplaceAll(path, "#", "%23")
	path = strings.ReplaceAll(path, "@", "%40")
	path = strings.ReplaceAll(path, "!", "%21")
	path = strings.ReplaceAll(path, "$", "%24")
	path = strings.ReplaceAll(path, " ", "%20")

	return path
}
