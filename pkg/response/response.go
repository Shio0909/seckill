package response

import (
	"net/http"
	"seckill/pkg/e"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Traceid string      `json:"traceid,omitempty"` //链路id
}

// Success 成功响应
// HTTP状态码一律返回200，CODE字段表示业务状态码
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    e.SUCCESS,
		Message: e.GetMsg(e.SUCCESS),
		Data:    data,
		Traceid: c.GetString("traceid"), //从上下文中获取traceid
	})
}

// Fail 失败响应
func Fail(c *gin.Context, code int) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: e.GetMsg(code),         //自动查表获取错误信息
		Traceid: c.GetString("traceid"), //从上下文中获取traceid
		Data:    nil,
	})
}

// FailWithMsg 失败响应，允许自定义错误信息
func FailWithMsg(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: msg,                    //使用自定义错误信息
		Traceid: c.GetString("traceid"), //从上下文中获取traceid
		Data:    nil,
	})
}

// FailWithData 失败响应，允许携带数据
func FailWithData(c *gin.Context, code int, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: e.GetMsg(code),         //自动查表获取错误信息
		Traceid: c.GetString("traceid"), //从上下文中获取traceid
		Data:    data,
	})
}
