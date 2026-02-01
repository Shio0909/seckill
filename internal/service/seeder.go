package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"seckill/internal/model"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/redis"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// 活动票务平台 - 种子数据
// 创建丰富的活动数据用于推荐系统测试
// 包含: 演唱会、体育赛事、展览、话剧、音乐节

// InitProductData 初始化活动数据
func InitProductData() {
	var count int64
	if err := database.DB.Model(&model.Product{}).Count(&count).Error; err != nil {
		logger.Log.Error("查询活动数量失败", zap.Error(err))
		return
	}

	if count > 0 {
		logger.Log.Info("活动数据已存在，跳过初始化", zap.Int64("count", count))
		return
	}

	logger.Log.Info("检测到数据库为空，正在初始化活动数据...")

	events := generateEventData()

	for _, event := range events {
		// 写入MySQL
		if err := database.DB.Create(&event).Error; err != nil {
			logger.Log.Error("初始化活动失败", zap.Error(err), zap.String("name", event.Name))
			continue
		}

		// 库存预热到Redis
		redisKey := fmt.Sprintf("seckill:stock:%d", event.ID)
		if err := redis.Client.Set(context.Background(), redisKey, event.Stock, 0).Err(); err != nil {
			logger.Log.Error("Redis库存预热失败", zap.Error(err), zap.String("key", redisKey))
			continue
		}

		// 设置热度分数到ZSet（用于热门召回）
		hotKey := fmt.Sprintf("hot:city:%d", event.CityID)
		redis.Client.ZAdd(context.Background(), hotKey, goredis.Z{
			Score:  event.HotScore,
			Member: event.ID,
		})
		redis.Client.ZAdd(context.Background(), "hot:all", goredis.Z{
			Score:  event.HotScore,
			Member: event.ID,
		})

		logger.Log.Debug("活动创建成功",
			zap.Uint("id", event.ID),
			zap.String("name", event.Name),
			zap.Int("stock", event.Stock),
		)
	}

	logger.Log.Info("活动数据初始化完成", zap.Int("count", len(events)))
}

// generateEventData 生成活动数据
func generateEventData() []model.Product {
	now := time.Now()
	events := []model.Product{}

	concerts := []struct {
		name   string
		artist string
		city   string
		cityID int
		venue  string
		price  float64
		high   float64
		stock  int
		hot    float64
	}{
		{"周杰伦 嘉年华巡回演唱会 上海站", "周杰伦", "上海", 1, "上海体育场", 580, 1680, 50000, 9999},
		{"周杰伦 嘉年华巡回演唱会 北京站", "周杰伦", "北京", 2, "鸟巢", 580, 1680, 60000, 9800},
		{"张学友 60+巡回演唱会 广州站", "张学友", "广州", 3, "天河体育中心", 480, 1580, 40000, 9500},
		{"林俊杰 JJ20巡回演唱会 深圳站", "林俊杰", "深圳", 4, "深圳大运中心", 380, 1280, 35000, 9200},
		{"五月天 诺亚方舟演唱会 成都站", "五月天", "成都", 5, "凤凰山体育公园", 480, 1480, 45000, 9000},
		{"薛之谦 天外来物巡演 杭州站", "薛之谦", "杭州", 6, "杭州奥体中心", 380, 1080, 30000, 8800},
		{"邓紫棋 演唱会 南京站", "邓紫棋", "南京", 7, "南京奥体中心", 380, 980, 25000, 8500},
		{"华晨宇 火星演唱会 武汉站", "华晨宇", "武汉", 8, "武汉体育中心", 380, 1080, 30000, 8300},
		{"陈奕迅 Fear and Dreams 重庆站", "陈奕迅", "重庆", 9, "重庆奥体中心", 480, 1380, 35000, 9100},
		{"Taylor Swift Eras Tour 上海站", "Taylor Swift", "上海", 1, "上海体育场", 880, 2680, 55000, 9950},
	}

	for i, c := range concerts {
		events = append(events, model.Product{
			Name:        c.name,
			CategoryID:  1,
			CityID:      c.cityID,
			City:        c.city,
			Venue:       c.venue,
			Price:       c.price,
			HighPrice:   c.high,
			Stock:       c.stock,
			Description: fmt.Sprintf("%s 2026年全国巡回演唱会，震撼来袭！", c.artist),
			ImageURL:    fmt.Sprintf("https://img.eventhub.com/concert/%d.jpg", i+1),
			Artist:      c.artist,
			Tags:        "演唱会,热门,巡演",
			HotScore:    c.hot,
			StartTime:   now.Add(time.Duration(i+1) * 24 * time.Hour),      // 开票时间
			EndTime:     now.Add(time.Duration(i+30) * 24 * time.Hour),     // 开票结束
			EventTime:   now.Add(time.Duration(i+60) * 24 * time.Hour),     // 活动时间
		})
	}

	sports := []struct {
		name   string
		artist string
		city   string
		cityID int
		venue  string
		price  float64
		stock  int
		hot    float64
	}{
		{"CBA总决赛 辽宁vs广东", "CBA", "沈阳", 10, "辽宁体育馆", 280, 10000, 8000},
		{"中超联赛 上海海港vs北京国安", "中超", "上海", 1, "上海体育场", 80, 30000, 7500},
		{"NBA中国赛 湖人vs篮网", "NBA", "上海", 1, "梅赛德斯奔驰文化中心", 580, 12000, 9200},
		{"F1中国大奖赛", "F1", "上海", 1, "上海国际赛车场", 680, 50000, 8800},
		{"世界杯预选赛 中国vs日本", "国足", "北京", 2, "鸟巢", 180, 60000, 8500},
		{"WTT乒乓球大满贯 北京站", "WTT", "北京", 2, "首都体育馆", 180, 8000, 7000},
	}

	for i, s := range sports {
		events = append(events, model.Product{
			Name:        s.name,
			CategoryID:  2,
			CityID:      s.cityID,
			City:        s.city,
			Venue:       s.venue,
			Price:       s.price,
			HighPrice:   s.price * 3,
			Stock:       s.stock,
			Description: fmt.Sprintf("2026年%s精彩对决，不容错过！", s.name),
			ImageURL:    fmt.Sprintf("https://img.eventhub.com/sports/%d.jpg", i+1),
			Artist:      s.artist,
			Tags:        "体育,赛事,热门",
			HotScore:    s.hot,
			StartTime:   now.Add(time.Duration(i+5) * 24 * time.Hour),
			EndTime:     now.Add(time.Duration(i+20) * 24 * time.Hour),
			EventTime:   now.Add(time.Duration(i+40) * 24 * time.Hour),
		})
	}

	exhibitions := []struct {
		name   string
		city   string
		cityID int
		venue  string
		price  float64
		stock  int
		hot    float64
	}{
		{"故宫博物院 清明上河图特展", "北京", 2, "故宫博物院", 60, 5000, 8500},
		{"上海博物馆 古埃及文明展", "上海", 1, "上海博物馆", 0, 8000, 8000},
		{"梵高沉浸式光影艺术展", "深圳", 4, "深圳当代艺术馆", 168, 3000, 7500},
		{"国家博物馆 丝绸之路文物展", "北京", 2, "中国国家博物馆", 30, 10000, 7000},
		{"成都博物馆 三星堆特展", "成都", 5, "成都博物馆", 50, 6000, 8200},
		{"浙江美术馆 宋代书画展", "杭州", 6, "浙江美术馆", 40, 4000, 6500},
	}

	for i, e := range exhibitions {
		events = append(events, model.Product{
			Name:        e.name,
			CategoryID:  3,
			CityID:      e.cityID,
			City:        e.city,
			Venue:       e.venue,
			Price:       e.price,
			HighPrice:   e.price,
			Stock:       e.stock,
			Description: fmt.Sprintf("2026年度重磅展览，%s邀您共赏！", e.name),
			ImageURL:    fmt.Sprintf("https://img.eventhub.com/exhibition/%d.jpg", i+1),
			Artist:      "",
			Tags:        "展览,文化,艺术",
			HotScore:    e.hot,
			StartTime:   now,
			EndTime:     now.Add(90 * 24 * time.Hour),
			EventTime:   now.Add(7 * 24 * time.Hour),
		})
	}

	dramas := []struct {
		name   string
		city   string
		cityID int
		venue  string
		price  float64
		stock  int
		hot    float64
	}{
		{"话剧《茶馆》 经典复排", "北京", 2, "北京人艺", 180, 1000, 7800},
		{"音乐剧《猫》中文版", "上海", 1, "上海大剧院", 280, 1500, 7500},
		{"开心麻花《乌龙山伯爵》", "北京", 2, "地质礼堂", 180, 800, 7200},
		{"赖声川《暗恋桃花源》", "上海", 1, "上海话剧艺术中心", 380, 600, 7000},
		{"孟京辉《恋爱的犀牛》", "北京", 2, "蜂巢剧场", 280, 400, 6800},
		{"国家大剧院《图兰朵》", "北京", 2, "国家大剧院", 480, 2000, 7600},
	}

	for i, d := range dramas {
		events = append(events, model.Product{
			Name:        d.name,
			CategoryID:  4,
			CityID:      d.cityID,
			City:        d.city,
			Venue:       d.venue,
			Price:       d.price,
			HighPrice:   d.price * 2,
			Stock:       d.stock,
			Description: fmt.Sprintf("经典剧目%s，感受舞台魅力！", d.name),
			ImageURL:    fmt.Sprintf("https://img.eventhub.com/drama/%d.jpg", i+1),
			Artist:      "",
			Tags:        "话剧,舞台剧,经典",
			HotScore:    d.hot,
			StartTime:   now.Add(time.Duration(i+3) * 24 * time.Hour),
			EndTime:     now.Add(time.Duration(i+30) * 24 * time.Hour),
			EventTime:   now.Add(time.Duration(i+15) * 24 * time.Hour),
		})
	}

	festivals := []struct {
		name   string
		city   string
		cityID int
		venue  string
		price  float64
		stock  int
		hot    float64
	}{
		{"草莓音乐节 北京站", "北京", 2, "北京世园公园", 380, 20000, 8800},
		{"迷笛音乐节 青岛站", "青岛", 11, "青岛世博园", 320, 15000, 8200},
		{"超级草莓音乐节 上海站", "上海", 1, "上海浦东", 420, 25000, 8600},
		{"仙人掌音乐节 成都站", "成都", 5, "成都露天音乐公园", 299, 12000, 7800},
		{"简单生活节 上海站", "上海", 1, "上海世博园", 380, 18000, 8000},
		{"泡泡岛音乐节 深圳站", "深圳", 4, "深圳欢乐港湾", 350, 15000, 7600},
	}

	for i, f := range festivals {
		events = append(events, model.Product{
			Name:        f.name,
			CategoryID:  5,
			CityID:      f.cityID,
			City:        f.city,
			Venue:       f.venue,
			Price:       f.price,
			HighPrice:   f.price * 1.5,
			Stock:       f.stock,
			Description: fmt.Sprintf("2026年%s，用音乐点燃夏天！", f.name),
			ImageURL:    fmt.Sprintf("https://img.eventhub.com/festival/%d.jpg", i+1),
			Artist:      "",
			Tags:        "音乐节,户外,夏日",
			HotScore:    f.hot,
			StartTime:   now.Add(time.Duration(i+10) * 24 * time.Hour),
			EndTime:     now.Add(time.Duration(i+25) * 24 * time.Hour),
			EventTime:   now.Add(time.Duration(i+45) * 24 * time.Hour),
		})
	}

	// 打乱顺序
	rand.Shuffle(len(events), func(i, j int) {
		events[i], events[j] = events[j], events[i]
	})

	return events
}
