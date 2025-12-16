package validator

// ========================================================================
// 【重点学习】参数校验器
// ========================================================================
// 为什么需要参数校验？
// 1. 防止恶意输入导致的安全问题（SQL注入、XSS等）
// 2. 尽早发现错误，减少无效的数据库/服务调用
// 3. 提供清晰的错误信息，提升用户体验
//
// gin 内置的 validator 是 go-playground/validator
// 通过 struct tag 定义校验规则，如 binding:"required,min=6"
// ========================================================================

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zhTranslations "github.com/go-playground/validator/v10/translations/zh"
)

var (
	Trans ut.Translator // 全局翻译器
)

// InitValidator 初始化校验器（在 main 中调用）
func InitValidator() {
	// 获取 gin 的 validator 引擎
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		// ====================================================================
		// 【重点学习】注册中文翻译器
		// ====================================================================
		// 默认错误信息是英文，生产环境需要中文
		// 使用 universal-translator 实现国际化
		// ====================================================================
		zhT := zh.New()
		uni := ut.New(zhT, zhT)
		Trans, _ = uni.GetTranslator("zh")
		zhTranslations.RegisterDefaultTranslations(v, Trans)

		// ====================================================================
		// 【重点学习】自定义字段名（用 json tag 替代字段名）
		// ====================================================================
		// 默认错误信息显示的是 Go 结构体字段名（如 Username）
		// 我们希望显示 json tag 中的名字（如 username）
		// ====================================================================
		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})

		// ====================================================================
		// 【重点学习】注册自定义校验规则
		// ====================================================================
		// 内置规则不够用时，可以注册自定义校验器
		// 如：手机号格式、身份证格式、密码强度等
		// ====================================================================
		v.RegisterValidation("mobile", validateMobile)
		v.RegisterValidation("password_strength", validatePasswordStrength)
	}
}

// ========================================================================
// 【重点学习】自定义校验函数
// ========================================================================
// 校验函数签名：func(fl validator.FieldLevel) bool
// fl.Field() 获取字段值
// 返回 true 表示校验通过，false 表示失败
// ========================================================================

// validateMobile 校验手机号格式
func validateMobile(fl validator.FieldLevel) bool {
	mobile := fl.Field().String()
	// 中国大陆手机号：1开头，第二位3-9，后面9位数字
	pattern := `^1[3-9]\d{9}$`
	matched, _ := regexp.MatchString(pattern, mobile)
	return matched
}

// validatePasswordStrength 校验密码强度
// 至少包含：8位以上，大写字母、小写字母、数字
func validatePasswordStrength(fl validator.FieldLevel) bool {
	password := fl.Field().String()
	if len(password) < 8 {
		return false
	}
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
	return hasUpper && hasLower && hasNumber
}

// ========================================================================
// 【重点学习】翻译校验错误
// ========================================================================
// 将 validator.ValidationErrors 翻译成中文 map
// 可以直接返回给前端展示
// ========================================================================

// TranslateErrors 翻译校验错误为中文
func TranslateErrors(err error) map[string]string {
	errs := make(map[string]string)
	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrors {
			// 使用翻译器翻译错误信息
			errs[e.Field()] = e.Translate(Trans)
		}
	}
	return errs
}

// GetFirstError 获取第一个错误信息（简化返回）
func GetFirstError(err error) string {
	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrors {
			return e.Translate(Trans)
		}
	}
	return err.Error()
}
