/*
 * 重点学习：统一响应格式封装
 *
 * 核心知识点：
 * 1. 标准化 API 响应：定义统一的 JSON 结构 (Code, Message, Data, TraceID)。
 * 2. 业务状态码 vs HTTP 状态码：
 *    - HTTP 状态码通常统一返回 200 OK (RESTful 风格除外)。
 *    - 业务状态码 (Code) 用于区分具体的业务结果（成功、参数错误、系统异常等）。
 * 3. 链路追踪集成：在响应中自动携带 TraceID，方便客户端排查问题。
 *
 * 面试高频问题：
 * Q1: 为什么前后端分离项目中需要统一响应格式？
 * A1: 降低沟通成本，前端可以封装统一的拦截器处理逻辑（如 Code!=0 统一弹窗报错）。
 *
 * Q2: HTTP 状态码全部返回 200 合理吗？
 * A2: 这是一个争议点。
 *     - 纯 RESTful 风格推荐使用 4xx/5xx 表示错误。
 *     - 业务开发中，为了避免网关或浏览器对非 200 请求的特殊处理（如重试、拦截），
 *       很多大厂（如微信接口）倾向于 HTTP 200 + 业务 Code 的模式。
 */
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
