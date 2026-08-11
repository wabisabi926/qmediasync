package controllers

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/disk"
)

type DirResp struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// GetPathList 获取目录列表
// @Summary 获取目录列表
// @Description 按同步源类型获取本地、OpenList或115的目录列表
// @Tags 路径管理
// @Accept json
// @Produce json
// @Param parent_id query string false "父目录ID，仅115使用"
// @Param parent_path query string false "父目录路径，本地或OpenList使用"
// @Param source_type query integer true "同步源类型，0-本地 1-115 2-OpenList"
// @Param account_id query integer false "账号ID，115或OpenList必填"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /path/list [get]
// @Security JwtAuth
// @Security ApiKeyAuth
func GetPathList(c *gin.Context) {
	type dirListReq struct {
		ParentId   string            `json:"parent_id" form:"parent_id"`
		ParentPath string            `json:"parent_path" form:"parent_path"`
		SourceType models.SourceType `json:"source_type" form:"source_type"`
		AccountId  uint              `json:"account_id" form:"account_id"`
	}
	var req dirListReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	ctx := c.Request.Context()
	var pathes []DirResp
	var err error
	switch req.SourceType {
	case models.SourceTypeLocal:
		pathes, err = GetLocalPath(req.ParentId)
	case models.SourceTypeOpenList:
		pathes, err = GetOpenListPath(ctx, req.ParentId, req.AccountId)
	case models.SourceType115:
		pathes, err = Get115PathList(ctx, req.ParentId, req.AccountId)
	case models.SourceTypeBaiduPan:
		pathes, err = GetBaiduPanPathList(ctx, req.ParentId, req.AccountId)
	default:
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "未知的同步源类型", Data: nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取目录列表失败: " + err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取目录列表成功", Data: pathes})
}

// 获取本地目录列表
// windows，如果parentPath为空，则获取盘符列表
// 非windows，如果parentPath为空，则获取根目录/的子目录列表
// FnOS环境下，parentPath必须在白名单目录内才允许访问
func GetLocalPath(parentPath string) ([]DirResp, error) {
	pathes := make([]DirResp, 0)
	if parentPath == "" {
		if runtime.GOOS == "windows" {
			// 获取盘符列表
			partitions, err := disk.Partitions(false)
			if err != nil {
				helpers.AppLogger.Errorf("获取盘符失败：%v", err)
				return nil, err
			}
			for _, partition := range partitions {
				pathes = append(pathes, DirResp{
					Id:   partition.Mountpoint + "\\",
					Name: partition.Mountpoint,
					Path: partition.Mountpoint + "\\",
				})
			}
			return pathes, nil
		} else {
			if helpers.IsFnOS {
				// 飞牛环境下，使用环境变量来获取有权限的目录
				if helpers.AccessiblePathes == "" {
					helpers.AccessiblePathes = os.Getenv("TRIM_DATA_ACCESSIBLE_PATHS")
				}
				helpers.SharePathes = os.Getenv("TRIM_DATA_SHARE_PATHS")
				helpers.AppLogger.Debugf("AccessiblePathes: %s, SharePathes: %s", helpers.AccessiblePathes, helpers.SharePathes)
				if helpers.AccessiblePathes != "" || helpers.SharePathes != "" {
					accessiblePaths := helpers.AccessiblePathes
					sharePaths := helpers.SharePathes
					if sharePaths != "" {
						accessiblePaths += ":" + sharePaths
					}
					paths := strings.Split(accessiblePaths, ":")
					for _, path := range paths {
						path = strings.TrimSpace(path)
						pathes = append(pathes, DirResp{
							Id:   path,
							Name: path,
							Path: path,
						})
					}
				}
				return pathes, nil
			} else {
				parentPath = "/"
			}
		}
	}
	// FnOS环境下校验路径白名单，防止越权访问
	if helpers.IsFnOS && parentPath != "/" {
		if !isPathAllowed(parentPath) {
			return nil, fmt.Errorf("无权限访问该目录: %s", parentPath)
		}
	}
	// 获取子目录列表
	entries, err := os.ReadDir(parentPath)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			// 跳过隐藏目录
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			fullPath := filepath.ToSlash(filepath.Join(parentPath, entry.Name()))
			pathes = append(pathes, DirResp{
				Id:   fullPath,
				Name: entry.Name(),
				Path: fullPath,
			})
		}
	}

	return pathes, nil
}

// isPathAllowed 检查路径是否在FnOS白名单目录内
func isPathAllowed(path string) bool {
	allowedPaths := make([]string, 0)
	if helpers.AccessiblePathes != "" {
		allowedPaths = append(allowedPaths, strings.Split(helpers.AccessiblePathes, ":")...)
	}
	if helpers.SharePathes != "" {
		allowedPaths = append(allowedPaths, strings.Split(helpers.SharePathes, ":")...)
	}
	cleanPath := filepath.Clean(path)
	for _, ap := range allowedPaths {
		ap = strings.TrimSpace(filepath.Clean(ap))
		if ap == "" {
			continue
		}
		if cleanPath == ap || strings.HasPrefix(cleanPath, ap+"/") {
			return true
		}
	}
	return false
}

func GetOpenListPath(ctx context.Context, parentPath string, accountId uint) ([]DirResp, error) {
	account, err := models.GetAccountById(accountId)
	if err != nil {
		return nil, err
	}
	// 去掉parentPath末尾的/
	parentPath = strings.TrimSuffix(parentPath, "/")
	parentPath = strings.TrimSuffix(parentPath, "\\")

	helpers.AppLogger.Debugf("获取OpenList目录列表, 父目录路径: %s", parentPath)
	client := account.GetOpenListClient()
	resp, err := client.FileList(ctx, parentPath, 1, 500)
	if err != nil {
		return nil, err
	}
	// 只返回文件夹列表
	folders := make([]DirResp, 0)
	for _, item := range resp.Content {
		if item.IsDir {
			folders = append(folders, DirResp{
				Id:   parentPath + "/" + item.Name,
				Name: item.Name,
				Path: parentPath + "/" + item.Name,
			})
		}
	}
	return folders, nil
}

func Get115PathList(ctx context.Context, parentId string, accountId uint) ([]DirResp, error) {
	account, err := models.GetAccountById(accountId)
	if err != nil {
		return nil, err
	}
	client := account.Get115Client()
	helpers.AppLogger.Debugf("获取115目录列表, 父目录ID: %s", parentId)
	// showFile=false: 只拉目录，减少响应体积；limit=1000: 覆盖大目录场景
	resp, err := client.GetFsList(ctx, parentId, true, false, true, 0, 1000)
	if err != nil {
		helpers.AppLogger.Warnf("获取115目录列表失败: 父目录：%s, 错误:%v", parentId, err)
		return nil, err
	}
	helpers.AppLogger.Debugf("成功获取115目录列表, 父目录ID: %s, 子目录数量: %d", parentId, len(resp.Data))
	folders := make([]DirResp, 0)
	for _, item := range resp.Data {
		folders = append(folders, DirResp{
			Id:   item.FileId,
			Name: item.FileName,
			Path: filepath.ToSlash(filepath.Join(resp.PathStr, item.FileName)),
		})
	}
	return folders, nil
}

func GetBaiduPanPathList(ctx context.Context, parentId string, accountId uint) ([]DirResp, error) {
	account, err := models.GetAccountById(accountId)
	if err != nil {
		return nil, err
	}
	client := account.GetBaiDuPanClient()
	fileList, fileErr := client.GetFileList(ctx, parentId, 1, 1, 0, 1000)
	if fileErr != nil {
		helpers.AppLogger.Warnf("获取百度网盘目录列表失败: 父目录：%s, 错误:%v", parentId, fileErr)
		return nil, fileErr
	}
	items := make([]DirResp, 0)
	for _, item := range fileList {
		// 去掉item.Path开头的/
		item.Path = strings.TrimPrefix(item.Path, "/")
		items = append(items, DirResp{
			Id:   item.Path,
			Name: filepath.Base(item.Path),
			Path: item.Path,
		})
	}
	return items, nil
}

// 创建文件夹
func CreateDir(c *gin.Context) {
	type createDirReq struct {
		ParentId   string            `json:"parent_id" form:"parent_id"`
		ParentPath string            `json:"parent_path" form:"parent_path"`
		SourceType models.SourceType `json:"source_type" form:"source_type"`
		AccountId  uint              `json:"account_id" form:"account_id"`
		Name       string            `json:"name" form:"name"`
	}
	var req createDirReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "文件夹名称不能为空", Data: nil})
		return
	}
	// 文件夹名称不能包含/
	if strings.Contains(req.Name, "/") {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "文件夹名称不能包含/", Data: nil})
		return
	}
	ctx := c.Request.Context()
	var err error
	var pathId string
	switch req.SourceType {
	case models.SourceTypeLocal:
		pathId, err = makeLocalPath(req.ParentId, req.Name)
	case models.SourceTypeOpenList:
		pathId, err = makeOpenListPath(req.ParentId, req.Name, req.AccountId)
	case models.SourceType115:
		pathId, err = make115PathList(ctx, req.ParentId, req.ParentPath, req.Name, req.AccountId)
	case models.SourceTypeBaiduPan:
		pathId, err = makeBaiduPanPathList(ctx, req.ParentId, req.Name, req.AccountId)
	default:
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "未知的同步源类型", Data: nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "创建目录失败: " + err.Error(), Data: nil})
		return
	}
	dirResp := DirResp{
		Id:   pathId,
		Name: req.Name,
		Path: filepath.ToSlash(filepath.Join(req.ParentPath, req.Name)),
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "创建目录成功", Data: dirResp})
}

// 创建本地目录
func makeLocalPath(parentId string, folderName string) (string, error) {
	// 检查父目录是否存在
	if !helpers.PathExists(parentId) || parentId == "" {
		return "", fmt.Errorf("父目录不存在: %s", parentId)
	}
	// 构建新目录路径
	newDir := filepath.Join(parentId, folderName)
	// 创建目录
	if err := os.Mkdir(newDir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %s, 错误: %v", newDir, err)
	}
	return newDir, nil
}

// 创建openlist目录
func makeOpenListPath(parentId string, folderName string, accountId uint) (string, error) {
	if parentId == "" {
		parentId = "/"
	}
	// 检查父目录是否存在
	account, err := models.GetAccountById(accountId)
	if err != nil {
		return "", fmt.Errorf("获取账号失败: %v", err)
	}
	client := account.GetOpenListClient()
	_, err = client.FileDetail(parentId)
	if err != nil {
		return "", fmt.Errorf("获取openlist目录详情失败，目录可能不存在: %v", err)
	}
	newDir := filepath.ToSlash(filepath.Join(parentId, folderName))
	err = client.Mkdir(newDir)
	if err != nil {
		return "", fmt.Errorf("创建openlist目录失败: %s, 错误: %v", newDir, err)
	}
	return newDir, nil
}

// 创建115目录
func make115PathList(ctx context.Context, parentId, parentPath, folderName string, accountId uint) (string, error) {
	if parentId == "" {
		parentId = "0"
	}
	// 检查父目录是否存在
	account, err := models.GetAccountById(accountId)
	if err != nil {
		return "", fmt.Errorf("获取账号失败: %v", err)
	}
	client := account.Get115Client()
	if parentId != "0" {
		_, err = client.GetFsDetailByCid(ctx, parentId)
		if err != nil {
			return "", fmt.Errorf("获取115目录详情失败，目录可能不存在: %v", err)
		}
	}
	newDir := filepath.ToSlash(filepath.Join(parentPath, folderName))
	newPathId, err := client.MkDir(ctx, parentId, folderName)
	if err != nil {
		return "", fmt.Errorf("创建115目录失败: %s, 错误: %v", newDir, err)
	}
	return newPathId, nil
}

func makeBaiduPanPathList(ctx context.Context, parentId string, folderName string, accountId uint) (string, error) {
	if parentId == "" {
		parentId = "/"
	}
	// 检查父目录是否存在
	account, err := models.GetAccountById(accountId)
	if err != nil {
		return "", fmt.Errorf("获取账号失败: %v", err)
	}
	client := account.GetBaiDuPanClient()
	exists, err := client.PathExists(ctx, parentId)
	if err != nil {
		return "", fmt.Errorf("获取百度网盘目录失败，目录可能不存在: %v", err)
	}
	if !exists {
		return "", fmt.Errorf("父目录不存在: %s", parentId)
	}
	// 创建新目录
	newDir := filepath.ToSlash(filepath.Join(parentId, folderName))
	err = client.Mkdir(ctx, newDir)
	if err != nil {
		return "", fmt.Errorf("创建百度网盘目录失败: %s, 错误: %v", newDir, err)
	}
	return newDir, nil
}
