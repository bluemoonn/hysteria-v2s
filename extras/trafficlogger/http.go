package trafficlogger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/apernet/hysteria/core/v2/server"
)

const (
	indexHTML = `<!DOCTYPE html><html lang="en"><head> <meta charset="UTF-8"> <meta name="viewport" content="width=device-width, initial-scale=1.0"> <title>Hysteria Traffic Stats API Server</title> <style>body{font-family: Arial, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; padding: 0; background-color: #f4f4f4;}.container{padding: 20px; background-color: #fff; box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1); border-radius: 5px;}</style></head><body> <div class="container"> <p>This is a Hysteria Traffic Stats API server.</p><p>Check the documentation for usage.</p></div></body></html>`
)

// TrafficStatsServer implements both server.TrafficLogger and http.Handler
// to provide a simple HTTP API to get the traffic stats per user.
type TrafficStatsServer interface {
	server.TrafficLogger
	http.Handler
	PushSystemStatusInterval(url string, interval time.Duration)
}

// trafficStatsServerImpl 用于管理系统状态提交的结构体
type trafficStatsServerImpl struct {
	Mutex     sync.RWMutex
	StatsMap  map[string]*trafficStatsEntry
	OnlineMap map[string]int
	KickMap   map[string]struct{}
	Secret    string
}

type trafficStatsEntry struct {
	Tx uint64 `json:"tx"`
	Rx uint64 `json:"rx"`
}

type TrafficPushEntry struct {
	UserID int64 `json:"user_id"`
	U      int64 `json:"u"`
	D      int64 `json:"d"`
}

type TrafficPushRequest struct {
	Data []TrafficPushEntry `json:"data"`
}

// SystemStatus 用于表示系统状态
type SystemStatus struct {
	Cpu    string `json:"cpu"`
	Mem    string `json:"mem"`
	Disk   string `json:"disk"`
	Uptime uint64 `json:"uptime"`
}

func NewTrafficStatsServer(secret string) TrafficStatsServer {
	return &trafficStatsServerImpl{
		StatsMap:  make(map[string]*trafficStatsEntry),
		KickMap:   make(map[string]struct{}),
		OnlineMap: make(map[string]int),
		Secret:    secret,
	}
}

// GetSystemInfo 获取系统状态信息
func GetSystemInfo() (Cpu string, Mem string, Disk string, Uptime uint64, err error) {
	errorString := ""

	cpuPercent, err := cpu.Percent(0, false)
	if len(cpuPercent) > 0 && err == nil {
		Cpu = fmt.Sprintf("%.0f%%", cpuPercent[0])
	} else {
		Cpu = "0%"
		errorString += fmt.Sprintf("获取CPU使用率失败: %s ", err)
	}

	memUsage, err := mem.VirtualMemory()
	if err != nil {
		errorString += fmt.Sprintf("获取内存使用率失败: %s ", err)
	} else {
		Mem = fmt.Sprintf("%.0f%%", memUsage.UsedPercent)
	}

	diskUsage, err := disk.Usage("/")
	if err != nil {
		errorString += fmt.Sprintf("获取磁盘使用率失败: %s ", err)
	} else {
		Disk = fmt.Sprintf("%.0f%%", diskUsage.UsedPercent)
	}

	uptime, err := host.Uptime()
	if err != nil {
		errorString += fmt.Sprintf("获取系统运行时间失败: %s ", err)
	} else {
		Uptime = uptime
	}

	if errorString != "" {
		err = fmt.Errorf(errorString)
	}

	return Cpu, Mem, Disk, Uptime, err
}

// PushSystemStatusInterval 定期提交系统状态
func (s *trafficStatsServerImpl) PushSystemStatusInterval(url string, interval time.Duration) {
	fmt.Println("系统状态监控已启动")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := s.PushSystemStatus(url); err != nil {
			fmt.Println("系统状态信息提交失败:", err)
		}
	}
}

// PushSystemStatus 向指定的URL提交系统状态信息
func (s *trafficStatsServerImpl) PushSystemStatus(url string) error {
	s.Mutex.Lock()         // 写锁，阻止其他操作的并发访问
	defer s.Mutex.Unlock() // 确保在函数退出时释放写锁

	cpu, mem, disk, uptime, err := GetSystemInfo()
	if err != nil {
		return err
	}

	status := SystemStatus{
		Cpu:    cpu,
		Mem:    mem,
		Disk:   disk,
		Uptime: uptime,
	}

	// 将请求对象转换为 JSON
	jsonData, err := json.Marshal(status)
	if err != nil {
		return err
	}

	// 发起 HTTP 请求并提交数据
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 检查 HTTP 响应状态，处理错误等
	if resp.StatusCode != http.StatusOK {
		return errors.New("HTTP请求失败，状态码: " + resp.Status)
	}

	return nil
}

// PushTrafficToV2RaySocksInterval 定时提交用户流量情况
func (s *trafficStatsServerImpl) PushTrafficToV2RaySocksInterval(url string, interval time.Duration) {
	fmt.Println("用户流量情况监控已启动")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := s.PushTrafficToV2RaySocks(url); err != nil {
			fmt.Println("用户流量信息提交失败:", err)
		}
	}
}

// PushTrafficToV2RaySocks 向v2raysocks 提交用户流量使用情况
func (s *trafficStatsServerImpl) PushTrafficToV2RaySocks(url string) error {
	s.Mutex.Lock()         // 写锁，阻止其他操作 StatsMap 的并发访问
	defer s.Mutex.Unlock() // 确保在函数退出时释放写锁

	// 创建一个请求对象并填充数据
	request := TrafficPushRequest{
		Data: []TrafficPushEntry{},
	}
	for id, stats := range s.StatsMap {
		userID, err := strconv.ParseInt(id, 10, 64) // 假设 id 是字符串类型，需要转换为 int64
		if err != nil {
			return err
		}
		request.Data = append(request.Data, TrafficPushEntry{
			UserID: userID,
			U:      int64(stats.Tx),
			D:      int64(stats.Rx),
		})
	}
	// 如果不存在数据则跳过
	if len(request.Data) == 0 {
		return nil
	}

	// 将请求对象转换为 JSON
	jsonData, err := json.Marshal(request.Data)
	if err != nil {
		return err
	}

	// 发起 HTTP 请求并提交数据
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println(resp)
		return err
	}
	defer resp.Body.Close()

	// 检查 HTTP 响应状态，处理错误等
	if resp.StatusCode != http.StatusOK {
		return errors.New("HTTP request failed with status code: " + resp.Status)
	}

	// 清空流量记录
	s.StatsMap = make(map[string]*trafficStatsEntry)

	return nil
}

func (s *trafficStatsServerImpl) LogTraffic(id string, tx, rx uint64) (ok bool) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	_, ok = s.KickMap[id]
	if ok {
		delete(s.KickMap, id)
		return false
	}

	entry, ok := s.StatsMap[id]
	if !ok {
		entry = &trafficStatsEntry{}
		s.StatsMap[id] = entry
	}
	entry.Tx += tx
	entry.Rx += rx

	return true
}

// LogOnlineStateChanged updates the online state to the online map.
func (s *trafficStatsServerImpl) LogOnlineState(id string, online bool) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	if online {
		s.OnlineMap[id]++
	} else {
		s.OnlineMap[id]--
		if s.OnlineMap[id] <= 0 {
			delete(s.OnlineMap, id)
		}
	}
}

func (s *trafficStatsServerImpl) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.Secret != "" && r.Header.Get("Authorization") != s.Secret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		_, _ = w.Write([]byte(indexHTML))
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/traffic" {
		s.getTraffic(w, r)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/kick" {
		s.kick(w, r)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/online" {
		s.getOnline(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *trafficStatsServerImpl) getTraffic(w http.ResponseWriter, r *http.Request) {
	bClear, _ := strconv.ParseBool(r.URL.Query().Get("clear"))
	var jb []byte
	var err error
	if bClear {
		s.Mutex.Lock()
		jb, err = json.Marshal(s.StatsMap)
		s.StatsMap = make(map[string]*trafficStatsEntry)
		s.Mutex.Unlock()
	} else {
		s.Mutex.RLock()
		jb, err = json.Marshal(s.StatsMap)
		s.Mutex.RUnlock()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(jb)
}

func (s *trafficStatsServerImpl) getOnline(w http.ResponseWriter, r *http.Request) {
	s.Mutex.RLock()
	defer s.Mutex.RUnlock()

	jb, err := json.Marshal(s.OnlineMap)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(jb)
}

func (s *trafficStatsServerImpl) kick(w http.ResponseWriter, r *http.Request) {
	var ids []string
	err := json.NewDecoder(r.Body).Decode(&ids)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.Mutex.Lock()
	for _, id := range ids {
		s.KickMap[id] = struct{}{}
	}
	s.Mutex.Unlock()

	w.WriteHeader(http.StatusOK)
}

// 踢出用户名单
func (s *trafficStatsServerImpl) NewKick(id string) bool {
	s.Mutex.Lock()
	s.KickMap[id] = struct{}{}
	s.Mutex.Unlock()
	return true
}

// 确保 trafficStatsServerImpl 实现了 TrafficStatsServer 接口
var _ TrafficStatsServer = &trafficStatsServerImpl{}