package service

import (
	"errors"
	"seckill/internal/model"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/utils"

	"go.uber.org/zap"
)

// Register用户注册
func Register(username, password, phone string) error {
	//1、检查手机号或者用户名是否已经存在
	var count int64
	//用户名
	database.DB.Model(&model.User{}).Where("username=?", username).Count(&count)
	if count > 0 {
		logger.Log.Warn("用户注册失败，用户名已存在",
			zap.String("username", username),
		)
		return errors.New("用户名已存在")
	}
	//手机号
	database.DB.Model(&model.User{}).Where("phone=?", phone).Count(&count)
	if count > 0 {
		logger.Log.Warn("用户注册失败，手机号已存在",
			zap.String("phone", phone),
		)
		return errors.New("手机号已存在")
	}
	//2、对密码进行加密
	hashPwd, err := utils.HashPassword(password)
	if err != nil {
		//系统内部错误，加密失败
		logger.Log.Error("用户注册失败，密码加密错误", zap.Error(err))
		return errors.New("系统内部错误，请稍后再试")
	}
	//3、保存用户信息到数据库
	user := model.User{
		Username: username,
		Password: hashPwd,
		Phone:    phone,
		Status:   1,
		Avatar:   "http://image.test.com/default.jpg", // 默认头像
	}
	if err := database.DB.Create(&user).Error; err != nil {
		logger.Log.Error("用户注册失败，数据库错误", zap.Error(err))
		return errors.New("系统内部错误，请稍后再试")
	}
	logger.Log.Info("用户注册成功",
		zap.Uint("uid", user.ID),
		zap.String("username", username),
	)
	return nil
}

// Login用户登录
func Login(username, password string) (string, error) {
	var user model.User
	//1、根据用户名查询用户信息
	if err := database.DB.Where("username=?", username).First(&user).Error; err != nil {
		return "", errors.New("用户不存在")
	}
	//2、对比密码是否正确
	if !utils.CheckPasswordHash(password, user.Password) {
		logger.Log.Warn("用户登录失败，密码错误", zap.String("username", username))
		return "", errors.New("账号或密码错误")
	}
	//3、检查用户状态
	if user.Status != 1 {
		logger.Log.Warn("用户登录失败，用户被禁用", zap.String("username", username))
		return "", errors.New("用户已被禁用")
	}

	//4、颁发token
	token, err := utils.GenerateToken(user.ID, username)
	if err != nil {
		logger.Log.Error("用户登录失败，Token生成错误", zap.Error(err))
		return "", errors.New("系统内部错误，请稍后再试")
	}
	//成功登录
	logger.Log.Info("用户登录成功", zap.String("username", username))
	return token, nil
}
