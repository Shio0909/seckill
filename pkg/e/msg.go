package e

var MsgFlags = map[int]string{
	SUCCESS:        "ok",
	INVALID_PARAMS: "请求参数错误",
	ERROR:          "fail",

	// 用户模块 10000开头
	ERROR_EXIST_USER:            "用户已存在",
	ERROR_NOT_EXIST_USER:        "用户不存在",
	ERROR_PASSWORD_WRONG:        "密码错误",
	ERROR_TOKEN_GENERATE:        "token生成失败",
	ERROR_TOKEN_INVALID:         "无效的token",
	ERROR_TOKEN_EXPIRE:          "token已过期",
	ERROR_AUTH_CHECK_TOKEN:      "Token鉴权失败",
	ERROR_AUTH_CHECK_HEADER:     "Token头信息错误",
	ERROR_AUTH_CHECK_PERMISSION: "没有权限访问该资源",

	//商品模块 20000开头
	ERROR_NOT_EXIST_PRODUCT: "商品不存在",
	ERROR_PRODUCT_STOCK:     "商品库存不足",
	ERROR_ALREADY_PURCHASED: "已购买过该商品",
}

// GetMsg 获取状态码对应的信息
func GetMsg(code int) string {
	msg, ok := MsgFlags[code]
	if ok {
		return msg
	}
	return MsgFlags[ERROR]
}
