package recall

import "context"

// RecallItem 召回物品
type RecallItem struct {
	EventID int64   `json:"event_id"`
	Score   float64 `json:"score"`
	Source  string  `json:"source"` // 召回来源: item_cf, user_cf, hot, vector, tag
}

// Recaller 召回器接口
type Recaller interface {
	// Name 召回器名称
	Name() string
	// Recall 执行召回
	Recall(ctx context.Context, req *RecallRequest) ([]RecallItem, error)
	// Weight 召回权重
	Weight() float64
}

// RecallRequest 召回请求
type RecallRequest struct {
	UserID     int64    `json:"user_id"`
	City       string   `json:"city"`
	CategoryID int64    `json:"category_id"`
	Count      int      `json:"count"`
	Exclude    []int64  `json:"exclude"`    // 需要排除的物品ID
	History    []int64  `json:"history"`    // 用户历史行为
	Tags       []string `json:"tags"`       // 用户偏好标签
}

// RecallResponse 召回响应
type RecallResponse struct {
	Items     []RecallItem      `json:"items"`
	DebugInfo map[string]string `json:"debug_info,omitempty"`
}

// Config 召回配置
type Config struct {
	ItemCF ItemCFConfig `yaml:"itemCF"`
	UserCF UserCFConfig `yaml:"userCF"`
	Hot    HotConfig    `yaml:"hot"`
	Vector VectorConfig `yaml:"vector"`
	Tag    TagConfig    `yaml:"tag"`
}

// ItemCFConfig ItemCF配置
type ItemCFConfig struct {
	Enabled bool    `yaml:"enabled"`
	Weight  float64 `yaml:"weight"`
	TopK    int     `yaml:"topK"`
}

// UserCFConfig UserCF配置
type UserCFConfig struct {
	Enabled bool    `yaml:"enabled"`
	Weight  float64 `yaml:"weight"`
	TopK    int     `yaml:"topK"`
}

// HotConfig 热门召回配置
type HotConfig struct {
	Enabled bool    `yaml:"enabled"`
	Weight  float64 `yaml:"weight"`
	TopK    int     `yaml:"topK"`
}

// VectorConfig 向量召回配置
type VectorConfig struct {
	Enabled        bool    `yaml:"enabled"`
	Weight         float64 `yaml:"weight"`
	TopK           int     `yaml:"topK"`
	MilvusHost     string  `yaml:"milvusHost"`
	MilvusPort     int     `yaml:"milvusPort"`
	CollectionName string  `yaml:"collectionName"`
}

// TagConfig 标签召回配置
type TagConfig struct {
	Enabled bool    `yaml:"enabled"`
	Weight  float64 `yaml:"weight"`
	TopK    int     `yaml:"topK"`
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		ItemCF: ItemCFConfig{Enabled: true, Weight: 0.30, TopK: 100},
		UserCF: UserCFConfig{Enabled: true, Weight: 0.20, TopK: 100},
		Hot:    HotConfig{Enabled: true, Weight: 0.20, TopK: 200},
		Vector: VectorConfig{Enabled: true, Weight: 0.20, TopK: 100, MilvusHost: "localhost", MilvusPort: 19530, CollectionName: "event_embeddings"},
		Tag:    TagConfig{Enabled: true, Weight: 0.10, TopK: 100},
	}
}
