package nsqlookupd

/**
 实现一个数据结构，用来存储topic ,channel,client 等组成的拓扑信息。所有的数据都存放在内存之中
*/
import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

/*
  定义该结构
*/
type RegistrationDB struct {
	sync.RWMutex
	registrationMap map[Registration]Producers
}

/*
 定义一个注册号 ，注册号为一个抽象的概念
*/
type Registration struct {
	Category string
	Key      string
	SubKey   string
}

/*
定义注册号的复数形式(数组)，朱主要为了方便使用
*/
type Registrations []Registration

/*
 端信息，但实际代指什么还要结合其他代码
*/
type PeerInfo struct {
	lastUpdate       int64
	id               string
	RemoteAddress    string `json:"remote_address"`
	Hostname         string `json:"hostname"`
	BroadcastAddress string `json:"broadcast_address"`
	TCPPort          int    `json:"tcp_port"`
	HTTPPort         int    `json:"http_port"`
	Version          string `json:"version"`
}
/*
 生产者
*/
type Producer struct {
	peerInfo     *PeerInfo // 生产者端信息
	tombstoned   bool // 是否冻结
	tombstonedAt time.Time //冻结时间
}

// 定义生产者的复数形式
type Producers []*Producer

// 实现生产者的序列化方法
func (p *Producer) String() string {
	return fmt.Sprintf("%s [%d, %d]", p.peerInfo.BroadcastAddress, p.peerInfo.TCPPort, p.peerInfo.HTTPPort)
}
// 实现生产者的冻结接口
func (p *Producer) Tombstone() {
	p.tombstoned = true
	p.tombstonedAt = time.Now()
}
//检查生产者是否冻结
func (p *Producer) IsTombstoned(lifetime time.Duration) bool {
	return p.tombstoned && time.Now().Sub(p.tombstonedAt) < lifetime
}

func NewRegistrationDB() *RegistrationDB {
	return &RegistrationDB{
		registrationMap: make(map[Registration]Producers),
	}
}

// add a registration key
func (r *RegistrationDB) AddRegistration(k Registration) {
	r.Lock()
	defer r.Unlock()
	_, ok := r.registrationMap[k]
	if !ok {
		r.registrationMap[k] = Producers{}
	}
}

// add a producer to a registration
// 如果本来就存在，就是什么也不做，返回false,否则返回true
func (r *RegistrationDB) AddProducer(k Registration, p *Producer) bool {
	r.Lock()
	defer r.Unlock()
	producers := r.registrationMap[k]
	found := false
	for _, producer := range producers {
		if producer.peerInfo.id == p.peerInfo.id {
			found = true
		}
	}
	if found == false {
		r.registrationMap[k] = append(producers, p)
	}
	return !found
}

// remove a producer from a registration
// 从一个注册号上删除一个producer 
func (r *RegistrationDB) RemoveProducer(k Registration, id string) (bool, int) {
	r.Lock()
	defer r.Unlock()
	producers, ok := r.registrationMap[k]
	if !ok {
		return false, 0
	}
	removed := false
	cleaned := Producers{}
	for _, producer := range producers {
		if producer.peerInfo.id != id {
			cleaned = append(cleaned, producer) //不断的生成新的指针数组，这样的操作真的ok吗？
		} else {
			removed = true
		}
	}
	// Note: this leaves keys in the DB even if they have empty lists
	r.registrationMap[k] = cleaned
	return removed, len(cleaned)
}

// remove a Registration and all it's producers
func (r *RegistrationDB) RemoveRegistration(k Registration) {
	r.Lock()
	defer r.Unlock()
	delete(r.registrationMap, k)
}

// * 代表可以匹配任意字符串
func (r *RegistrationDB) needFilter(key string, subkey string) bool {
	return key == "*" || subkey == "*"
}
//返回匹配的注册号
func (r *RegistrationDB) FindRegistrations(category string, key string, subkey string) Registrations {
	r.RLock()
	defer r.RUnlock()
	// 如果没有通配符，那么结果只有一个，查找可以更直接一点 
	if !r.needFilter(key, subkey) {
		k := Registration{category, key, subkey}
		if _, ok := r.registrationMap[k]; ok {
			return Registrations{k}
		}
		return Registrations{}
	}
	//如果有通配符，则需要找出所有匹配的注册号
	results := Registrations{}
	for k := range r.registrationMap {
		if !k.IsMatch(category, key, subkey) {
			continue
		}
		results = append(results, k)
	}
	return results
}
//通过注册号查找匹配的Producer
func (r *RegistrationDB) FindProducers(category string, key string, subkey string) Producers {
	r.RLock()
	defer r.RUnlock()
	if !r.needFilter(key, subkey) {
		k := Registration{category, key, subkey}
		return r.registrationMap[k]
	}

	results := Producers{}
	for k, producers := range r.registrationMap {
		if !k.IsMatch(category, key, subkey) {
			continue
		}
		for _, producer := range producers {
			found := false
			for _, p := range results {
				if producer.peerInfo.id == p.peerInfo.id {
					found = true
				}
			}
			if found == false {
				results = append(results, producer)
			}
		}
	}
	return results
}
//查找与某id(PeerInfo.id)绑定的所有注册号
func (r *RegistrationDB) LookupRegistrations(id string) Registrations {
	r.RLock()
	defer r.RUnlock()
	results := Registrations{}
	for k, producers := range r.registrationMap {
		for _, p := range producers {
			if p.peerInfo.id == id {
				results = append(results, k)
				break
			}
		}
	}
	return results
}
//检查注册号是否匹配
func (k Registration) IsMatch(category string, key string, subkey string) bool {
	if category != k.Category {
		return false
	}
	if key != "*" && k.Key != key {
		return false
	}
	if subkey != "*" && k.SubKey != subkey {
		return false
	}
	return true
}
// 返回匹配的注册号集合
func (rr Registrations) Filter(category string, key string, subkey string) Registrations {
	output := Registrations{}
	for _, k := range rr {
		if k.IsMatch(category, key, subkey) {
			output = append(output, k)
		}
	}
	return output
}
// 返回所有的Key组成的集合（未去重）
func (rr Registrations) Keys() []string {
	keys := make([]string, len(rr))
	for i, k := range rr {
		keys[i] = k.Key
	}
	return keys
}
// 返回所有的subKey组成的集合 (未去重)
func (rr Registrations) SubKeys() []string {
	subkeys := make([]string, len(rr))
	for i, k := range rr {
		subkeys[i] = k.SubKey
	}
	return subkeys
}
// 返回指定时间返回内的活跃生产者列表
func (pp Producers) FilterByActive(inactivityTimeout time.Duration, tombstoneLifetime time.Duration) Producers {
	now := time.Now()
	results := Producers{}
	for _, p := range pp {
		cur := time.Unix(0, atomic.LoadInt64(&p.peerInfo.lastUpdate))
		if now.Sub(cur) > inactivityTimeout || p.IsTombstoned(tombstoneLifetime) {
			continue
		}
		results = append(results, p)
	}
	return results
}

func (pp Producers) PeerInfo() []*PeerInfo {
	results := []*PeerInfo{}
	for _, p := range pp {
		results = append(results, p.peerInfo)
	}
	return results
}
