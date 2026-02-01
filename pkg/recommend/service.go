package recommend

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"seckill/pkg/feature"
	"seckill/pkg/recall"
	"seckill/pkg/rerank"
	pb "seckill/proto/ranking"
)

// Service 推荐服务
// Go端负责: 召回 + 特征读取 + 重排
// Python端负责: 排序模型推理
type Service struct {
	// 召回
	recallMerger *recall.Merger

	// 特征
	featureStore *feature.Store

	// 重排
	rerankPipeline *rerank.Pipeline

	// 排序模型gRPC客户端
	rankingClient pb.RankingServiceClient
	rankingConn   *grpc.ClientConn

	// 配置
	config *Config
}

// Config 推荐服务配置
type Config struct {
	RecallConfig      *recall.Config
	RankingServerAddr string
	MMRLambda         float64
	DefaultCount      int
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		RecallConfig:      recall.DefaultConfig(),
		RankingServerAddr: "localhost:50052",
		MMRLambda:         0.7,
		DefaultCount:      20,
	}
}

// NewService 创建推荐服务
func NewService(rdb *redis.Client, cfg *Config) (*Service, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 创建召回器
	itemCF := recall.NewItemCFRecall(rdb, cfg.RecallConfig.ItemCF)
	hot := recall.NewHotRecall(rdb, cfg.RecallConfig.Hot)

	var recallers []recall.Recaller
	recallers = append(recallers, itemCF, hot)

	// 向量召回(可选)
	if cfg.RecallConfig.Vector.Enabled {
		vector, err := recall.NewVectorRecall(cfg.RecallConfig.Vector)
		if err == nil {
			recallers = append(recallers, vector)
		}
	}

	merger := recall.NewMerger(rdb, recallers...)

	// 创建特征存储
	featureStore := feature.NewStore(rdb)

	// 创建重排管道
	filter := rerank.NewBusinessFilter(rdb)
	diversifier := rerank.NewMMRDiversifier(cfg.MMRLambda)
	rerankPipeline := rerank.NewPipeline(filter, diversifier)

	// 连接排序服务
	rankingConn, err := grpc.Dial(
		cfg.RankingServerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect ranking service failed: %w", err)
	}

	return &Service{
		recallMerger:   merger,
		featureStore:   featureStore,
		rerankPipeline: rerankPipeline,
		rankingClient:  pb.NewRankingServiceClient(rankingConn),
		rankingConn:    rankingConn,
		config:         cfg,
	}, nil
}

// RecommendRequest 推荐请求
type RecommendRequest struct {
	UserID     int64
	Scene      string // home, category, similar
	City       string
	CategoryID int64
	Count      int
	Debug      bool
}

// RecommendItem 推荐结果
type RecommendItem struct {
	EventID      int64   `json:"event_id"`
	Score        float64 `json:"score"`
	RecallSource string  `json:"recall_source"`
	DebugInfo    string  `json:"debug_info,omitempty"`
}

// RecommendResponse 推荐响应
type RecommendResponse struct {
	Items     []RecommendItem   `json:"items"`
	RequestID string            `json:"request_id"`
	LatencyMs int64             `json:"latency_ms"`
	DebugInfo map[string]string `json:"debug_info,omitempty"`
}

// Recommend 执行推荐
func (s *Service) Recommend(ctx context.Context, req *RecommendRequest) (*RecommendResponse, error) {
	startTime := time.Now()

	if req.Count <= 0 {
		req.Count = s.config.DefaultCount
	}

	// 1. 召回阶段 (Go)
	recallReq := &recall.RecallRequest{
		UserID:     req.UserID,
		City:       req.City,
		CategoryID: req.CategoryID,
		Count:      req.Count * 20, // 召回20倍候选
	}

	recallResult, err := s.recallMerger.Merge(ctx, recallReq)
	if err != nil {
		return nil, fmt.Errorf("recall failed: %w", err)
	}

	if len(recallResult.Items) == 0 {
		return &RecommendResponse{
			Items:     []RecommendItem{},
			LatencyMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 2. 特征读取 (Go)
	eventIDs := make([]int64, len(recallResult.Items))
	for i, item := range recallResult.Items {
		eventIDs[i] = item.EventID
	}

	userFeatures, _ := s.featureStore.GetUserFeatures(ctx, req.UserID)
	eventFeatures, _ := s.featureStore.BatchGetEventFeatures(ctx, eventIDs)

	// 3. 排序阶段 (Python gRPC)
	rankItems := make([]*pb.RankItem, 0, len(recallResult.Items))
	for _, item := range recallResult.Items {
		ef := eventFeatures[item.EventID]
		features := s.featureStore.BuildFeatureVector(userFeatures, ef)

		rankItems = append(rankItems, &pb.RankItem{
			EventId:      item.EventID,
			RecallScore:  item.Score,
			RecallSource: item.Source,
			Features:     features,
		})
	}

	rankResp, err := s.rankingClient.Rank(ctx, &pb.RankRequest{
		UserId: req.UserID,
		Items:  rankItems,
	})
	if err != nil {
		// 排序失败,降级使用召回分数
		rankResp = &pb.RankResponse{Items: rankItems}
	}

	// 4. 重排阶段 (Go)
	rerankItems := make([]rerank.RerankItem, 0, len(rankResp.Items))
	for _, item := range rankResp.Items {
		featMap := make(map[string]float64)
		if ef, ok := eventFeatures[item.EventId]; ok && ef != nil {
			featMap["category_id"] = float64(ef.CategoryID)
			featMap["city_id"] = float64(ef.CityID)
			featMap["price"] = ef.Price
		}

		rerankItems = append(rerankItems, rerank.RerankItem{
			EventID:      item.EventId,
			Score:        item.RankScore,
			RecallSource: item.RecallSource,
			Features:     featMap,
		})
	}

	rerankReq := &rerank.RerankRequest{
		UserID:  req.UserID,
		Count:   req.Count,
		City:    req.City,
		History: recallReq.History,
	}

	finalItems, err := s.rerankPipeline.Execute(ctx, rerankItems, rerankReq)
	if err != nil {
		return nil, fmt.Errorf("rerank failed: %w", err)
	}

	// 5. 构建响应
	response := &RecommendResponse{
		Items:     make([]RecommendItem, 0, len(finalItems)),
		LatencyMs: time.Since(startTime).Milliseconds(),
	}

	for _, item := range finalItems {
		respItem := RecommendItem{
			EventID:      item.EventID,
			Score:        item.Score,
			RecallSource: item.RecallSource,
		}
		if req.Debug {
			respItem.DebugInfo = fmt.Sprintf("score=%.4f", item.Score)
		}
		response.Items = append(response.Items, respItem)
	}

	if req.Debug {
		response.DebugInfo = map[string]string{
			"recall_count": fmt.Sprintf("%d", len(recallResult.Items)),
			"rank_count":   fmt.Sprintf("%d", len(rankResp.Items)),
			"final_count":  fmt.Sprintf("%d", len(finalItems)),
		}
		for source, count := range recallResult.SourceStats {
			response.DebugInfo["recall_"+source] = fmt.Sprintf("%d", count)
		}
	}

	return response, nil
}

// RecordBehavior 记录用户行为
func (s *Service) RecordBehavior(ctx context.Context, userID, eventID int64, behaviorType string) error {
	// 记录到召回器(用户历史)
	if err := s.recallMerger.RecordBehavior(ctx, userID, eventID); err != nil {
		return err
	}

	// 更新特征
	return s.featureStore.RecordBehavior(ctx, userID, eventID, behaviorType)
}

// Close 关闭服务
func (s *Service) Close() error {
	if s.rankingConn != nil {
		return s.rankingConn.Close()
	}
	return nil
}
