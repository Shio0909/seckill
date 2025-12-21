package handler

import (
	"context"
	"errors"

	"seckill/internal/model"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/utils"
	pb "seckill/proto/user"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ========================================================================
// 【重点学习】User 微服务 gRPC Handler 实现
// ========================================================================
// 这是 User 服务的核心实现，实现了 proto/user.proto 中定义的接口。
//
// 📝 简历亮点：
// - gRPC 服务实现模式
// - 业务逻辑与通信协议分离
// - 统一错误处理（gRPC Status）
//
// 🔥 面试高频：
// Q: gRPC 错误处理与 HTTP 有什么区别？
// A: gRPC 使用 Status 对象，包含 Code 和 Message，比 HTTP 状态码更丰富。
//    常用 codes: OK, InvalidArgument, NotFound, Internal, Unauthenticated
//
// Q: 为什么业务逻辑要放在 Handler 而不是直接写在 Controller？
// A: 关注点分离：Handler 处理 gRPC 协议，Controller 处理 HTTP 协议，
//    业务逻辑可以复用，方便测试和维护。
// ========================================================================

// UserHandler 用户服务处理器
type UserHandler struct {
	pb.UnimplementedUserServiceServer // 嵌入未实现的服务，保证向前兼容
}

// NewUserHandler 创建用户处理器
func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

// Register 用户注册
// 【重点】gRPC 方法签名：context.Context 作为第一个参数，返回 (Response, error)
func (h *UserHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	// 参数校验
	if req.Username == "" || req.Password == "" || req.Phone == "" {
		return nil, status.Error(codes.InvalidArgument, "用户名、密码和手机号不能为空")
	}

	// 检查用户名是否已存在
	var count int64
	database.DB.Model(&model.User{}).Where("username = ?", req.Username).Count(&count)
	if count > 0 {
		logger.Log.Warn("用户注册失败，用户名已存在", zap.String("username", req.Username))
		return nil, status.Error(codes.AlreadyExists, "用户名已存在")
	}

	// 检查手机号是否已存在
	database.DB.Model(&model.User{}).Where("phone = ?", req.Phone).Count(&count)
	if count > 0 {
		logger.Log.Warn("用户注册失败，手机号已存在", zap.String("phone", req.Phone))
		return nil, status.Error(codes.AlreadyExists, "手机号已存在")
	}

	// 密码加密
	hashPwd, err := utils.HashPassword(req.Password)
	if err != nil {
		logger.Log.Error("用户注册失败，密码加密错误", zap.Error(err))
		return nil, status.Error(codes.Internal, "系统内部错误")
	}

	// 创建用户
	user := model.User{
		Username: req.Username,
		Password: hashPwd,
		Phone:    req.Phone,
		Status:   1,
		Avatar:   "http://image.test.com/default.jpg",
	}

	if err := database.DB.Create(&user).Error; err != nil {
		logger.Log.Error("用户注册失败，数据库错误", zap.Error(err))
		return nil, status.Error(codes.Internal, "系统内部错误")
	}

	logger.Log.Info("用户注册成功", zap.Uint("uid", user.ID), zap.String("username", req.Username))

	return &pb.RegisterResponse{
		UserId:  int64(user.ID),
		Message: "注册成功",
	}, nil
}

// Login 用户登录
func (h *UserHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	if req.Username == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "用户名和密码不能为空")
	}

	var user model.User
	if err := database.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "用户不存在")
	}

	// 验证密码
	if !utils.CheckPasswordHash(req.Password, user.Password) {
		logger.Log.Warn("用户登录失败，密码错误", zap.String("username", req.Username))
		return nil, status.Error(codes.Unauthenticated, "账号或密码错误")
	}

	// 检查用户状态
	if user.Status != 1 {
		logger.Log.Warn("用户登录失败，用户被禁用", zap.String("username", req.Username))
		return nil, status.Error(codes.PermissionDenied, "用户已被禁用")
	}

	// 生成 Token
	token, err := utils.GenerateToken(user.ID, user.Username)
	if err != nil {
		logger.Log.Error("用户登录失败，Token生成错误", zap.Error(err))
		return nil, status.Error(codes.Internal, "系统内部错误")
	}

	logger.Log.Info("用户登录成功", zap.String("username", req.Username))

	return &pb.LoginResponse{
		Token:  token,
		UserId: int64(user.ID),
	}, nil
}

// ValidateToken 验证 Token
// 【重点】这个方法通常被 Gateway 或其他服务调用来验证用户身份
func (h *UserHandler) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "Token 不能为空")
	}

	// 解析 Token
	claims, err := utils.ParseToken(req.Token)
	if err != nil {
		return &pb.ValidateTokenResponse{
			Valid:   false,
			Message: "Token 无效或已过期",
		}, nil
	}

	return &pb.ValidateTokenResponse{
		Valid:    true,
		UserId:   int64(claims.UserID),
		Username: claims.Username,
		Message:  "Token 有效",
	}, nil
}

// GetUser 获取用户信息
func (h *UserHandler) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.UserInfo, error) {
	if req.UserId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "用户ID无效")
	}

	var user model.User
	if err := database.DB.First(&user, req.UserId).Error; err != nil {
		return nil, status.Error(codes.NotFound, "用户不存在")
	}

	return &pb.UserInfo{
		UserId:   int64(user.ID),
		Username: user.Username,
		Phone:    user.Phone,
		Avatar:   user.Avatar,
		Status:   int32(user.Status),
	}, nil
}

// GetUserByUsername 根据用户名获取用户（内部方法，供其他服务调用）
func GetUserByUsername(username string) (*model.User, error) {
	var user model.User
	if err := database.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, errors.New("用户不存在")
	}
	return &user, nil
}
