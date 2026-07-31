package controllers

import (
	"Q115-STRM/fnosproxy"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetFnosProxyConfig 获取飞牛反代配置
func GetFnosProxyConfig(c *gin.Context) {
	svc := fnosproxy.GetService()
	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "获取飞牛反代配置成功",
		Data:    svc.Snapshot(c.Request),
	})
}

// UpdateFnosProxyConfig 更新飞牛反代配置
func UpdateFnosProxyConfig(c *gin.Context) {
	var req fnosproxy.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error()})
		return
	}
	svc := fnosproxy.GetService()
	cfg, err := svc.Update(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "更新飞牛反代配置成功",
		Data:    cfg,
	})
}

// TestFnosProxyConfig 测试飞牛影视地址连通性
func TestFnosProxyConfig(c *gin.Context) {
	var req fnosproxy.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error()})
		return
	}
	svc := fnosproxy.GetService()
	if err := svc.TestUpdate(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{
		Code:    Success,
		Message: "飞牛影视地址连接正常",
		Data:    gin.H{"ok": true},
	})
}
