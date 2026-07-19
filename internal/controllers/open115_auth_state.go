package controllers

import (
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/v115auth"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetOAuthStatus(c *gin.Context) {
	type oauthStatusReq struct {
		AccountId uint `form:"account_id"`
		State     string `form:"state"`
	}
	var req oauthStatusReq
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
	provider, ok := v115auth.GetOAuthProvider(authSource.Provider)
	if !ok {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "不支持的授权提供者", Data: nil})
		return
	}

	token, err := provider.Poll(c.Request.Context(), req.State)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询OAuth授权状态失败: " + err.Error(), Data: nil})
		return
	}

	if token.Done {
		if !save115OAuthToken(c, account, token) {
			return
		}
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "查询OAuth授权状态成功", Data: map[string]interface{}{
		"done": token.Done,
	}})
}

func save115OAuthToken(c *gin.Context, account *models.Account, token v115auth.OAuthTokenResult) bool {
	if !account.UpdateToken(token.AccessToken, token.RefreshToken, token.ExpiresIn) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "保存115访问凭证失败", Data: nil})
		return false
	}

	client := account.Get115Client()
	userInfo, err := client.UserInfo()
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取115用户信息失败: " + err.Error(), Data: nil})
		return false
	}

	if !account.UpdateUser(string(userInfo.UserId), userInfo.UserName) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新用户信息失败", Data: nil})
		return false
	}

	return true
}

func GetAccountAuthAction(c *gin.Context) {
	type authActionReq struct {
		AccountId uint `form:"account_id"`
	}
	var req authActionReq
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

	action := "unsupported"
	if authSource.SupportsPKCE() {
		action = "pkce"
	} else if authSource.SupportsOAuth() {
		action = "oauth"
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "", Data: map[string]string{
		"action": action,
	}})
}