# ![Hysteria 2](logo.svg)

# 支持对接V2RaySocks面板的Hysteria2后端

### 项目说明
本项目基于hysteria官方内核二次开发，添加了从V2RaySocks获取节点信息、用户鉴权信息与上报用户流量的功能。

### 示例配置
```
v2raysocks:
  apiHost: https://yourdomain.com/modules/addons/v2raysocks_nodes/web_api.php
  apiKey: v2raysocks_nodes
  nodeID: 节点ID
tls:
  type: tls
  cert: /etc/hysteria/tls.crt
  key: /etc/hysteria/tls.key
auth:
  type: v2raysocks
trafficStats:
  listen: 127.0.0.1:7653
acl: 
  inline: 
    - reject(10.0.0.0/8)
    - reject(172.16.0.0/12)
    - reject(192.168.0.0/16)
    - reject(127.0.0.0/8)
    - reject(fc00::/7)
```
> 其他配置完全与hysteria文档的一致，可以查看hysteria2官方文档 [点击查看](https://hysteria.network/zh/docs/getting-started/Installation/) 

### 快速启动
```
docker run -itd --restart=always  --network=host \
 -e apiHost=https://example.com \
 -e apiKey=xxxxxxxxxxxxxxxxxxxxx \
 -e domain=hy2.example.com  \
 -e nodeID=1 \
ghcr.io/thh1451/hysteria:latest
```
### docker 仓库
```
docker pull ghcr.io/thh1451/hysteria:latest
```
