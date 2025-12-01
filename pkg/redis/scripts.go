package redis

import "github.com/redis/go-redis/v9"

//seckill LuaScript 秒杀脚本
//SeckillScript定义全局脚本变量

var SeckillScript *redis.Script

//脚本内容(秒杀核心逻辑)
//key【1】库存key
//key【2】用户key
//arg【1】用户id
const seckillLua = `
	--阶段1、防刷/幂等校验	
	--检查用户是否在已购买集合中
	if redis.call('sismember', KEYS[2], ARGV[1]) == 1 then
		return -1 --返回-用户已购买
	end
	--阶段2、库存校验
	--获取当前库存
	local stock = tonumber(redis.call('get', KEYS[1]))
	--判断库存是否充足
	if stock <= 0 then
		return -2 --返回-库存不足
	end
	--阶段3、扣减库存/记录购买用户
	--扣减库存
	redis.call('decr', KEYS[1]) --库存-1
	--将用户加入已购买集合
	redis.call('sadd', KEYS[2], ARGV[1])
	--返回成功
	return 1 --返回1表示抢购成功
`

//初始化脚本 需要在main函数启动时调用
func InitLuaScripts() {
	SeckillScript = redis.NewScript(seckillLua)
}
