package controllers

import (
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/v115auth"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetAppIdSources(c *gin.Context) {
	type appIdReq struct {
		Keyword string `form:"keyword"`
		Offset  int    `form:"offset"`
		Limit   int    `form:"limit"`
	}
	var req appIdReq
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}

	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 100 {
		req.Limit = 100
	}

	items, total, err := v115auth.SearchAppIDs(req.Keyword, req.Offset, req.Limit)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询APP ID失败: " + err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "查询成功", Data: map[string]interface{}{
		"items": items,
		"total": total,
	}})
}

func GetAccountAuthInfo(c *gin.Context) {
	type authInfoReq struct {
		AccountId uint `form:"account_id"`
	}
	var req authInfoReq
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}

	account, err := models.GetAccountById(req.AccountId)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
		return
	}

	authSource := account.V115AuthSource()

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "查询成功", Data: map[string]interface{}{
		"source_type":      account.SourceType,
		"app_id":          account.AppId,
		"app_id_name":     account.AppIdName,
		"auth_source_type": account.AuthSourceType,
		"auth_provider":    account.AuthProvider,
		"supports_pkce":    authSource.SupportsPKCE(),
		"supports_oauth":   authSource.SupportsOAuth(),
	}})
}