package auth

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/apernet/hysteria/core/v2/server"
)

var _ server.Authenticator = &V2RaySocksApiProvider{}

type V2RaySocksApiProvider struct {
	Client *http.Client
	URL    string
	Etag   string // 添加一个字段用于存储服务器返回的ETag
}

// 用户列表
var (
	usersMap map[string]User
	lock     sync.Mutex
)

type User struct {
	ID          int    `json:"id"`
	UUID        string `json:"uuid"`
	DeviceLimit int    `json:"dt"`
	SpeedLimit  int    `json:"st"`
}

type ResponseData struct {
	Users []User `json:"users"`
}

func getUserList(url string, etag string) ([]User, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, etag, nil
	}

	var responseData ResponseData
	err = json.NewDecoder(resp.Body).Decode(&responseData)
	if err != nil {
		return nil, "", err
	}

	newEtag := resp.Header.Get("ETag")
	return responseData.Users, newEtag, nil
}

func UpdateUsers(url string, interval time.Duration, trafficlogger server.TrafficLogger) {
	fmt.Println("用户列表自动更新服务已激活")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var etag string

	// 立即执行一次 getUserList
	userList, newEtag, err := getUserList(url, etag)
	if err != nil {
		fmt.Println("Error:", err)
		return // 直接返回，不进入循环
	}

	// 处理首次获取的用户列表
	if newEtag != "" && newEtag != etag {
		etag = newEtag

		lock.Lock()
		newUsersMap := make(map[string]User)
		for _, user := range userList {
			newUsersMap[user.UUID] = user
		}
		if trafficlogger != nil {
			for uuid := range usersMap {
				if _, exists := newUsersMap[uuid]; !exists {
					trafficlogger.LogOnlineState(strconv.Itoa(usersMap[uuid].ID), false)
				}
			}
		}

		usersMap = newUsersMap
		lock.Unlock()
	}

	for range ticker.C {
		userList, newEtag, err := getUserList(url, etag)
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}

		if newEtag != "" && newEtag != etag {
			etag = newEtag

			lock.Lock()
			newUsersMap := make(map[string]User)
			for _, user := range userList {
				newUsersMap[user.UUID] = user
			}
			if trafficlogger != nil {
				for uuid := range usersMap {
					if _, exists := newUsersMap[uuid]; !exists {
						trafficlogger.LogOnlineState(strconv.Itoa(usersMap[uuid].ID), false)
					}
				}
			}

			usersMap = newUsersMap
			lock.Unlock()
		}
	}
}

func getResponseEtag(url string, etag string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return etag, nil // Return the existing etag if no changes
	}

	newEtag := resp.Header.Get("ETag")
	return newEtag, nil
}

func CheckRemoteConf(url string, interval time.Duration) {
	fmt.Println("远程配置文件监控服务已激活")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var etag string

	for range ticker.C {
		newEtag, err := getResponseEtag(url, etag)
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}
		// 如果etag不同，则进行相应的操作
		if etag != "" && newEtag != etag {
			fmt.Println("远程配置文件已更改，程序即将重启...")
			// 创建一个重新启动的命令
			cmd := exec.Command(os.Args[0], os.Args[1:]...) // os.Args[0] 是当前程序的路径，os.Args[1:] 是传递给程序的参数

			// 启动新进程
			err := cmd.Start()
			if err != nil {
				fmt.Println("启动新进程失败:", err)
				return
			}

			// 退出当前程序
			os.Exit(0)
		} else {
			// 更新etag值
			etag = newEtag
		}
	}
}

// 验证代码
func (v *V2RaySocksApiProvider) Authenticate(addr net.Addr, auth string, tx uint64) (ok bool, id string) {

	// 获取判断连接用户是否在用户列表内
	lock.Lock()
	defer lock.Unlock()

	if user, exists := usersMap[auth]; exists {
		return true, strconv.Itoa(user.ID)
	}
	return false, ""
}
