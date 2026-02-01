package search

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// SearchHandler 搜索Handler
type SearchHandler struct {
	engine *SearchEngine
}

// NewSearchHandler 创建SearchHandler
func NewSearchHandler(engine *SearchEngine) *SearchHandler {
	return &SearchHandler{engine: engine}
}

// RegisterRoutes 注册路由
func (h *SearchHandler) RegisterRoutes(r *gin.RouterGroup) {
	search := r.Group("/search")
	{
		search.GET("", h.Search)
		search.GET("/suggest", h.Suggest)
		search.GET("/hot", h.HotKeywords)
	}
}

// Search 搜索接口
// @Summary 搜索活动
// @Description 根据关键词、分类、城市等条件搜索活动
// @Tags 搜索
// @Accept json
// @Produce json
// @Param keyword query string false "搜索关键词"
// @Param category query string false "分类"
// @Param city query string false "城市"
// @Param min_price query number false "最低价格"
// @Param max_price query number false "最高价格"
// @Param start_date query string false "开始日期 YYYY-MM-DD"
// @Param end_date query string false "结束日期 YYYY-MM-DD"
// @Param tags query string false "标签，逗号分隔"
// @Param sort_by query string false "排序方式：relevance, price_asc, price_desc, time, hot"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} SearchResponse
// @Router /api/v1/search [get]
func (h *SearchHandler) Search(c *gin.Context) {
	req := &SearchRequest{
		Keyword:   c.Query("keyword"),
		Category:  c.Query("category"),
		City:      c.Query("city"),
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
		SortBy:    c.Query("sort_by"),
	}

	if minPrice := c.Query("min_price"); minPrice != "" {
		if v, err := strconv.ParseFloat(minPrice, 64); err == nil {
			req.MinPrice = v
		}
	}

	if maxPrice := c.Query("max_price"); maxPrice != "" {
		if v, err := strconv.ParseFloat(maxPrice, 64); err == nil {
			req.MaxPrice = v
		}
	}

	if page := c.Query("page"); page != "" {
		if v, err := strconv.Atoi(page); err == nil {
			req.Page = v
		}
	}

	if pageSize := c.Query("page_size"); pageSize != "" {
		if v, err := strconv.Atoi(pageSize); err == nil {
			req.PageSize = v
		}
	}

	resp, err := h.engine.Search(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "搜索失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": resp,
	})
}

// Suggest 搜索建议
// @Summary 搜索建议
// @Description 根据输入前缀返回搜索建议
// @Tags 搜索
// @Accept json
// @Produce json
// @Param q query string true "搜索前缀"
// @Param size query int false "返回数量" default(10)
// @Success 200 {array} Suggestion
// @Router /api/v1/search/suggest [get]
func (h *SearchHandler) Suggest(c *gin.Context) {
	prefix := c.Query("q")
	if prefix == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数q不能为空",
		})
		return
	}

	size := 10
	if sizeStr := c.Query("size"); sizeStr != "" {
		if v, err := strconv.Atoi(sizeStr); err == nil {
			size = v
		}
	}

	suggestions, err := h.engine.Suggest(c.Request.Context(), prefix, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "获取建议失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": suggestions,
	})
}

// HotKeywords 热门搜索词
// @Summary 热门搜索词
// @Description 获取热门搜索关键词
// @Tags 搜索
// @Accept json
// @Produce json
// @Param size query int false "返回数量" default(10)
// @Success 200 {array} string
// @Router /api/v1/search/hot [get]
func (h *SearchHandler) HotKeywords(c *gin.Context) {
	// 这里可以从Redis获取热门搜索词
	// 暂时返回静态数据
	hotKeywords := []string{
		"周杰伦",
		"五月天",
		"演唱会",
		"话剧",
		"音乐节",
		"脱口秀",
		"相声",
		"展览",
		"儿童剧",
		"芭蕾舞",
	}

	size := 10
	if sizeStr := c.Query("size"); sizeStr != "" {
		if v, err := strconv.Atoi(sizeStr); err == nil && v > 0 && v < len(hotKeywords) {
			size = v
		}
	}

	if size > len(hotKeywords) {
		size = len(hotKeywords)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": hotKeywords[:size],
	})
}
