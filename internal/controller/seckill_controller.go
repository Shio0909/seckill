package controller

import (
	"net/http"
	"seckill/internal/service"
	"seckill/pkg/logger"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SeckillController 负责处理秒杀相关请求
type SeckillController struct{}

// Buy 处理秒杀请求
// @Summary 用户秒杀下单
// @Description 发起秒杀请求，扣减库存
// @Tags 秒杀模块
// @Accept x-www-form-urlencoded
// @Produce json
// @Security Bearer
// @Param product_id formData int true "商品ID"
// @Success 200 {object} map[string]interface{} "{"code":0,"msg":"抢购成功"}"
// @Router /api/seckill/buy [post]
func (sc *SeckillController) Buy(c *gin.Context) {
	//1、获取用户ID和商品ID
	//暂时模拟一个用户id
	uid, exists := c.Get("uid")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	userID := uid.(int) // 断言为 int
	//从请求参数获取商品ID
	pidStr := c.PostForm("product_id")
	productid, err := strconv.Atoi(pidStr)
	if err != nil {
		logger.Log.Error("获取商品ID失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的商品ID",
		})
		return
	}
	//2、调用service层的秒杀逻辑
	result, message := service.SeckillV2(userID, productid)
	//3、返回结果
	if result {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": message,
			"code":    0,
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": message,
			"code":    1,
		})
	}

}
