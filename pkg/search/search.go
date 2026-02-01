package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SearchConfig 搜索配置
type SearchConfig struct {
	ElasticURL   string
	IndexName    string
	Username     string
	Password     string
	MaxResults   int
	Timeout      time.Duration
}

// Event 活动/商品实体
type Event struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Category    string    `json:"category"`
	Venue       string    `json:"venue,omitempty"`
	City        string    `json:"city"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	Price       float64   `json:"price"`
	MinPrice    float64   `json:"min_price"`
	MaxPrice    float64   `json:"max_price"`
	Status      int       `json:"status"` // 0:未开售 1:预售中 2:售卖中 3:已售罄
	Tags        []string  `json:"tags,omitempty"`
	Artists     []string  `json:"artists,omitempty"`
	ImageURL    string    `json:"image_url,omitempty"`
	SalesCount  int64     `json:"sales_count"`
	ViewCount   int64     `json:"view_count"`
	Score       float64   `json:"score"` // 评分
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SearchRequest 搜索请求
type SearchRequest struct {
	Keyword    string   `json:"keyword"`
	Category   string   `json:"category,omitempty"`
	City       string   `json:"city,omitempty"`
	MinPrice   float64  `json:"min_price,omitempty"`
	MaxPrice   float64  `json:"max_price,omitempty"`
	StartDate  string   `json:"start_date,omitempty"`  // 开始日期 2024-01-01
	EndDate    string   `json:"end_date,omitempty"`    // 结束日期
	Tags       []string `json:"tags,omitempty"`
	SortBy     string   `json:"sort_by,omitempty"`     // relevance, price_asc, price_desc, time, hot
	Page       int      `json:"page,omitempty"`
	PageSize   int      `json:"page_size,omitempty"`
}

// SearchResponse 搜索响应
type SearchResponse struct {
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
	Events   []*Event `json:"events"`
	Took     int64    `json:"took"` // 耗时ms
}

// Suggestion 搜索建议
type Suggestion struct {
	Text      string `json:"text"`
	Type      string `json:"type"` // keyword, artist, venue, event
	Highlight string `json:"highlight,omitempty"`
}

// SearchEngine 搜索引擎
type SearchEngine struct {
	config SearchConfig
	client *http.Client
}

// NewSearchEngine 创建搜索引擎
func NewSearchEngine(config SearchConfig) *SearchEngine {
	if config.MaxResults == 0 {
		config.MaxResults = 100
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}
	if config.IndexName == "" {
		config.IndexName = "events"
	}

	return &SearchEngine{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
	}
}

// CreateIndex 创建索引
func (s *SearchEngine) CreateIndex(ctx context.Context) error {
	mapping := map[string]interface{}{
		"settings": map[string]interface{}{
			"number_of_shards":   3,
			"number_of_replicas": 1,
			"analysis": map[string]interface{}{
				"analyzer": map[string]interface{}{
					"ik_smart_pinyin": map[string]interface{}{
						"type":      "custom",
						"tokenizer": "ik_smart",
						"filter":    []string{"lowercase", "pinyin_filter"},
					},
				},
				"filter": map[string]interface{}{
					"pinyin_filter": map[string]interface{}{
						"type":                  "pinyin",
						"keep_full_pinyin":      false,
						"keep_joined_full_pinyin": true,
						"keep_original":         true,
						"limit_first_letter_length": 16,
						"remove_duplicated_term": true,
					},
				},
			},
		},
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"id":          map[string]string{"type": "long"},
				"title": map[string]interface{}{
					"type":     "text",
					"analyzer": "ik_max_word",
					"fields": map[string]interface{}{
						"pinyin": map[string]interface{}{
							"type":     "text",
							"analyzer": "ik_smart_pinyin",
						},
						"keyword": map[string]interface{}{
							"type": "keyword",
						},
					},
				},
				"description": map[string]interface{}{
					"type":     "text",
					"analyzer": "ik_smart",
				},
				"category":   map[string]string{"type": "keyword"},
				"venue":      map[string]string{"type": "keyword"},
				"city":       map[string]string{"type": "keyword"},
				"start_time": map[string]string{"type": "date"},
				"end_time":   map[string]string{"type": "date"},
				"price":      map[string]string{"type": "float"},
				"min_price":  map[string]string{"type": "float"},
				"max_price":  map[string]string{"type": "float"},
				"status":     map[string]string{"type": "integer"},
				"tags":       map[string]string{"type": "keyword"},
				"artists": map[string]interface{}{
					"type":     "text",
					"analyzer": "ik_smart",
					"fields": map[string]interface{}{
						"keyword": map[string]interface{}{
							"type": "keyword",
						},
					},
				},
				"image_url":   map[string]string{"type": "keyword", "index": "false"},
				"sales_count": map[string]string{"type": "long"},
				"view_count":  map[string]string{"type": "long"},
				"score":       map[string]string{"type": "float"},
				"created_at":  map[string]string{"type": "date"},
				"updated_at":  map[string]string{"type": "date"},
				"suggest": map[string]interface{}{
					"type":            "completion",
					"analyzer":        "ik_smart",
					"preserve_separators": true,
					"preserve_position_increments": true,
					"max_input_length": 50,
				},
			},
		},
	}

	return s.esRequest(ctx, "PUT", "/"+s.config.IndexName, mapping, nil)
}

// IndexEvent 索引活动
func (s *SearchEngine) IndexEvent(ctx context.Context, event *Event) error {
	// 构建suggest字段
	doc := map[string]interface{}{
		"id":          event.ID,
		"title":       event.Title,
		"description": event.Description,
		"category":    event.Category,
		"venue":       event.Venue,
		"city":        event.City,
		"start_time":  event.StartTime,
		"end_time":    event.EndTime,
		"price":       event.Price,
		"min_price":   event.MinPrice,
		"max_price":   event.MaxPrice,
		"status":      event.Status,
		"tags":        event.Tags,
		"artists":     event.Artists,
		"image_url":   event.ImageURL,
		"sales_count": event.SalesCount,
		"view_count":  event.ViewCount,
		"score":       event.Score,
		"created_at":  event.CreatedAt,
		"updated_at":  event.UpdatedAt,
		"suggest": map[string]interface{}{
			"input":  append([]string{event.Title}, event.Artists...),
			"weight": event.SalesCount / 100,
		},
	}

	endpoint := fmt.Sprintf("/%s/_doc/%d", s.config.IndexName, event.ID)
	return s.esRequest(ctx, "PUT", endpoint, doc, nil)
}

// BulkIndexEvents 批量索引
func (s *SearchEngine) BulkIndexEvents(ctx context.Context, events []*Event) error {
	var buf bytes.Buffer

	for _, event := range events {
		// action行
		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": s.config.IndexName,
				"_id":    event.ID,
			},
		}
		actionBytes, _ := json.Marshal(action)
		buf.Write(actionBytes)
		buf.WriteByte('\n')

		// document行
		doc := map[string]interface{}{
			"id":          event.ID,
			"title":       event.Title,
			"description": event.Description,
			"category":    event.Category,
			"venue":       event.Venue,
			"city":        event.City,
			"start_time":  event.StartTime,
			"price":       event.Price,
			"min_price":   event.MinPrice,
			"max_price":   event.MaxPrice,
			"status":      event.Status,
			"tags":        event.Tags,
			"artists":     event.Artists,
			"sales_count": event.SalesCount,
			"view_count":  event.ViewCount,
			"score":       event.Score,
			"suggest": map[string]interface{}{
				"input":  append([]string{event.Title}, event.Artists...),
				"weight": event.SalesCount / 100,
			},
		}
		docBytes, _ := json.Marshal(doc)
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}

	return s.esRequest(ctx, "POST", "/_bulk", nil, &buf)
}

// Search 搜索
func (s *SearchEngine) Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > s.config.MaxResults {
		req.PageSize = s.config.MaxResults
	}

	query := s.buildSearchQuery(req)

	var result map[string]interface{}
	endpoint := fmt.Sprintf("/%s/_search", s.config.IndexName)
	
	if err := s.esRequestWithResult(ctx, "POST", endpoint, query, &result); err != nil {
		return nil, err
	}

	return s.parseSearchResult(result, req.Page, req.PageSize)
}

// buildSearchQuery 构建搜索查询
func (s *SearchEngine) buildSearchQuery(req *SearchRequest) map[string]interface{}{
	must := []map[string]interface{}{}
	filter := []map[string]interface{}{}

	// 关键词搜索（多字段）
	if req.Keyword != "" {
		must = append(must, map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  req.Keyword,
				"fields": []string{"title^3", "title.pinyin^2", "description", "artists^2", "venue", "tags"},
				"type":   "best_fields",
				"fuzziness": "AUTO",
			},
		})
	}

	// 分类过滤
	if req.Category != "" {
		filter = append(filter, map[string]interface{}{
			"term": map[string]string{"category": req.Category},
		})
	}

	// 城市过滤
	if req.City != "" {
		filter = append(filter, map[string]interface{}{
			"term": map[string]string{"city": req.City},
		})
	}

	// 价格范围
	if req.MinPrice > 0 || req.MaxPrice > 0 {
		priceRange := map[string]interface{}{}
		if req.MinPrice > 0 {
			priceRange["gte"] = req.MinPrice
		}
		if req.MaxPrice > 0 {
			priceRange["lte"] = req.MaxPrice
		}
		filter = append(filter, map[string]interface{}{
			"range": map[string]interface{}{"min_price": priceRange},
		})
	}

	// 日期范围
	if req.StartDate != "" || req.EndDate != "" {
		dateRange := map[string]interface{}{}
		if req.StartDate != "" {
			dateRange["gte"] = req.StartDate
		}
		if req.EndDate != "" {
			dateRange["lte"] = req.EndDate
		}
		filter = append(filter, map[string]interface{}{
			"range": map[string]interface{}{"start_time": dateRange},
		})
	}

	// 标签过滤
	if len(req.Tags) > 0 {
		filter = append(filter, map[string]interface{}{
			"terms": map[string]interface{}{"tags": req.Tags},
		})
	}

	// 只显示在售状态
	filter = append(filter, map[string]interface{}{
		"terms": map[string]interface{}{"status": []int{1, 2}},
	})

	// 构建bool查询
	boolQuery := map[string]interface{}{}
	if len(must) > 0 {
		boolQuery["must"] = must
	} else {
		boolQuery["must"] = []map[string]interface{}{
			{"match_all": map[string]interface{}{}},
		}
	}
	if len(filter) > 0 {
		boolQuery["filter"] = filter
	}

	// 排序
	sort := s.buildSort(req.SortBy)

	// 分页
	from := (req.Page - 1) * req.PageSize

	return map[string]interface{}{
		"query": map[string]interface{}{
			"bool": boolQuery,
		},
		"sort":    sort,
		"from":    from,
		"size":    req.PageSize,
		"highlight": map[string]interface{}{
			"fields": map[string]interface{}{
				"title":       map[string]interface{}{},
				"description": map[string]interface{}{},
			},
			"pre_tags":  []string{"<em>"},
			"post_tags": []string{"</em>"},
		},
	}
}

// buildSort 构建排序
func (s *SearchEngine) buildSort(sortBy string) []map[string]interface{} {
	switch sortBy {
	case "price_asc":
		return []map[string]interface{}{
			{"min_price": map[string]string{"order": "asc"}},
		}
	case "price_desc":
		return []map[string]interface{}{
			{"min_price": map[string]string{"order": "desc"}},
		}
	case "time":
		return []map[string]interface{}{
			{"start_time": map[string]string{"order": "asc"}},
		}
	case "hot":
		return []map[string]interface{}{
			{"sales_count": map[string]string{"order": "desc"}},
			{"view_count": map[string]string{"order": "desc"}},
		}
	case "score":
		return []map[string]interface{}{
			{"score": map[string]string{"order": "desc"}},
		}
	default: // relevance
		return []map[string]interface{}{
			{"_score": map[string]string{"order": "desc"}},
			{"sales_count": map[string]string{"order": "desc"}},
		}
	}
}

// parseSearchResult 解析搜索结果
func (s *SearchEngine) parseSearchResult(result map[string]interface{}, page, pageSize int) (*SearchResponse, error) {
	resp := &SearchResponse{
		Page:     page,
		PageSize: pageSize,
		Events:   make([]*Event, 0),
	}

	// 获取总数
	if hits, ok := result["hits"].(map[string]interface{}); ok {
		if total, ok := hits["total"].(map[string]interface{}); ok {
			if value, ok := total["value"].(float64); ok {
				resp.Total = int64(value)
			}
		}

		// 获取命中文档
		if hitsList, ok := hits["hits"].([]interface{}); ok {
			for _, hit := range hitsList {
				hitMap := hit.(map[string]interface{})
				if source, ok := hitMap["_source"].(map[string]interface{}); ok {
					event := s.parseEvent(source)
					resp.Events = append(resp.Events, event)
				}
			}
		}
	}

	// 获取耗时
	if took, ok := result["took"].(float64); ok {
		resp.Took = int64(took)
	}

	return resp, nil
}

// parseEvent 解析活动
func (s *SearchEngine) parseEvent(source map[string]interface{}) *Event {
	event := &Event{}

	if v, ok := source["id"].(float64); ok {
		event.ID = int64(v)
	}
	if v, ok := source["title"].(string); ok {
		event.Title = v
	}
	if v, ok := source["description"].(string); ok {
		event.Description = v
	}
	if v, ok := source["category"].(string); ok {
		event.Category = v
	}
	if v, ok := source["venue"].(string); ok {
		event.Venue = v
	}
	if v, ok := source["city"].(string); ok {
		event.City = v
	}
	if v, ok := source["price"].(float64); ok {
		event.Price = v
	}
	if v, ok := source["min_price"].(float64); ok {
		event.MinPrice = v
	}
	if v, ok := source["max_price"].(float64); ok {
		event.MaxPrice = v
	}
	if v, ok := source["status"].(float64); ok {
		event.Status = int(v)
	}
	if v, ok := source["sales_count"].(float64); ok {
		event.SalesCount = int64(v)
	}
	if v, ok := source["view_count"].(float64); ok {
		event.ViewCount = int64(v)
	}
	if v, ok := source["score"].(float64); ok {
		event.Score = v
	}
	if v, ok := source["image_url"].(string); ok {
		event.ImageURL = v
	}

	// 解析标签
	if tags, ok := source["tags"].([]interface{}); ok {
		event.Tags = make([]string, len(tags))
		for i, tag := range tags {
			if t, ok := tag.(string); ok {
				event.Tags[i] = t
			}
		}
	}

	// 解析艺人
	if artists, ok := source["artists"].([]interface{}); ok {
		event.Artists = make([]string, len(artists))
		for i, artist := range artists {
			if a, ok := artist.(string); ok {
				event.Artists[i] = a
			}
		}
	}

	return event
}

// Suggest 搜索建议
func (s *SearchEngine) Suggest(ctx context.Context, prefix string, size int) ([]Suggestion, error) {
	if size <= 0 {
		size = 10
	}

	query := map[string]interface{}{
		"suggest": map[string]interface{}{
			"event-suggest": map[string]interface{}{
				"prefix": prefix,
				"completion": map[string]interface{}{
					"field":           "suggest",
					"size":            size,
					"skip_duplicates": true,
					"fuzzy": map[string]interface{}{
						"fuzziness": 1,
					},
				},
			},
		},
	}

	var result map[string]interface{}
	endpoint := fmt.Sprintf("/%s/_search", s.config.IndexName)
	
	if err := s.esRequestWithResult(ctx, "POST", endpoint, query, &result); err != nil {
		return nil, err
	}

	suggestions := make([]Suggestion, 0)

	if suggest, ok := result["suggest"].(map[string]interface{}); ok {
		if eventSuggest, ok := suggest["event-suggest"].([]interface{}); ok {
			for _, s := range eventSuggest {
				if sMap, ok := s.(map[string]interface{}); ok {
					if options, ok := sMap["options"].([]interface{}); ok {
						for _, opt := range options {
							if optMap, ok := opt.(map[string]interface{}); ok {
								suggestion := Suggestion{Type: "event"}
								if text, ok := optMap["text"].(string); ok {
									suggestion.Text = text
								}
								suggestions = append(suggestions, suggestion)
							}
						}
					}
				}
			}
		}
	}

	return suggestions, nil
}

// DeleteEvent 删除活动
func (s *SearchEngine) DeleteEvent(ctx context.Context, eventID int64) error {
	endpoint := fmt.Sprintf("/%s/_doc/%d", s.config.IndexName, eventID)
	return s.esRequest(ctx, "DELETE", endpoint, nil, nil)
}

// esRequest 发送ES请求
func (s *SearchEngine) esRequest(ctx context.Context, method, endpoint string, body interface{}, rawBody io.Reader) error {
	var result map[string]interface{}
	return s.esRequestWithResult(ctx, method, endpoint, body, &result)
}

// esRequestWithResult 发送ES请求并获取结果
func (s *SearchEngine) esRequestWithResult(ctx context.Context, method, endpoint string, body interface{}, result interface{}) error {
	var reqBody io.Reader

	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(data)
	}

	url := strings.TrimRight(s.config.ElasticURL, "/") + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if s.config.Username != "" {
		req.SetBasicAuth(s.config.Username, s.config.Password)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("ES请求失败: %d - %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		return json.Unmarshal(respBody, result)
	}

	return nil
}
