package validator

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
		// 默认错误信息是英文，生产环境需要中文
		// 使用 universal-translator 实现国际化
		zhT := zh.New()
		uni := ut.New(zhT, zhT)
		Trans, _ = uni.GetTranslator("zh")
		if err := zhTranslations.RegisterDefaultTranslations(v, Trans); err != nil {
			panic("注册中文翻译失败: " + err.Error())
		}

		// 默认错误信息显示的是 Go 结构体字段名（如 Username）
		// 我们希望显示 json tag 中的名字（如 username）
		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})

		// 内置规则不够用时，可以注册自定义校验器
		// 如：手机号格式、身份证格式、密码强度等
		if err := v.RegisterValidation("mobile", validateMobile); err != nil {
			panic("注册手机号校验器失败: " + err.Error())
		}
		if err := v.RegisterValidation("password_strength", validatePasswordStrength); err != nil {
			panic("注册密码强度校验器失败: " + err.Error())
		}
	}
}

// 校验函数签名：func(fl validator.FieldLevel) bool
// fl.Field() 获取字段值
// 返回 true 表示校验通过，false 表示失败

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

// 将 validator.ValidationErrors 翻译成中文 map
// 可以直接返回给前端展示

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
