package main

import "github.com/redis/go-redis/v9"

const prefix = "dispatch:"

var locationScript = redis.NewScript(`
local old=redis.call('HGET',KEYS[1],'h3_cell')
if old then redis.call('SREM',ARGV[1]..'cell:'..old,ARGV[2]) else
 redis.call('HSET',KEYS[1],'available','1','current_assignment_id','') end
redis.call('SADD',ARGV[1]..'cell:'..ARGV[3],ARGV[2])
redis.call('HSET',KEYS[1],'id',ARGV[2],'h3_cell',ARGV[3],'latitude',ARGV[4],'longitude',ARGV[5],'updated_at',ARGV[6])
return redis.call('HGETALL',KEYS[1])`)
var claimScript = redis.NewScript(`
local existing=redis.call('GET',KEYS[1])
if existing then
 if redis.call('HGET',ARGV[1]..'assignment:'..existing,'payload')~=ARGV[2] then return {'conflict'} end
 return {'existing',existing}
end
if redis.call('HGET',KEYS[2],'available')~='1' then return {'busy'} end
redis.call('HSET',KEYS[2],'available','0','current_assignment_id',ARGV[3],'updated_at',ARGV[5])
redis.call('HSET',KEYS[3],'record',ARGV[4],'payload',ARGV[2],'status','active','rider_id',ARGV[6],'completed_at','')
redis.call('SET',KEYS[1],ARGV[3])
return {'claimed',ARGV[3]}`)
var completeScript = redis.NewScript(`
if redis.call('EXISTS',KEYS[1])==0 then return {'missing'} end
local rid=redis.call('HGET',KEYS[1],'rider_id')
if redis.call('HGET',KEYS[1],'status')~='completed' then
 redis.call('HSET',KEYS[1],'status','completed','completed_at',ARGV[2])
 local rk=ARGV[1]..'rider:'..rid
 if redis.call('HGET',rk,'current_assignment_id')==ARGV[3] then
 redis.call('HSET',rk,'available','1','current_assignment_id','','updated_at',ARGV[2]) end
end
return {'completed',rid}`)
