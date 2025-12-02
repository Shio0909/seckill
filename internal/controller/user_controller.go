package controller

import (
	"seckill/internal/service"

	"github.com/gin-gonic/gin"
)

type UserController struct{}

// Register 用户注册
// @Summary 用户注册
// @Description 用户注册接口
// @Tags 用户模块
// @Accept json
// @Produce json
// @Param request body object{username=string,password=string,phone=string} true "注册参数"
// @Success 200 {object} map[string]interface{} "{"msg": "注册成功"}"
// @Router /api/register [post]
// Register 注册接口
func (uc *UserController) Register(c *gin.Context) {
	var form struct {
		Username string `form:"username" binding:"required"`
		Password string `form:"password" binding:"required"`
		Phone    string `form:"phone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := service.Register(form.Username, form.Password, form.Phone); err != nil {
		c.JSON(500, gin.H{"error": "注册失败: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "注册成功"})
}

// Login 用户登录
// @Summary 用户登录
// @Description 用户登录获取 Token
// @Tags 用户模块
// @Accept json
// @Produce json
// @Param request body object{username=string,password=string} true "登录参数"
// @Success 200 {object} map[string]interface{} "{"token": "eyJ...", "msg": "登录成功"}"
// @Router /api/login [post]
// Login 登录接口
func (uc *UserController) Login(c *gin.Context) {
	var form struct {
		Username string `form:"username" binding:"required"`
		Password string `form:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	token, err := service.Login(form.Username, form.Password)
	if err != nil {
		c.JSON(401, gin.H{"error": "登录失败: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "登录成功", "token": token})
}
