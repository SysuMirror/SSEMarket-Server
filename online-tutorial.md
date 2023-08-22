# Online Tutorial

## 仓库管理

上线版本的代码在仓库的`online`分支之中，若需修改，请在本地修改完成后，自行PR，审查通过后方可合并。

## 部署访问信息


```bash
# 阿里云服务器
ip 120.79.77.160
password SSE_Market

# sse_market_client_pc: 
http://120.79.77.160

# sse_market_client_mobile: 
http://120.79.77.160:81

# sse_market_manager: 
http://120.79.77.160:82

# sse_market_server: 
http://120.79.77.160:8080

# mysql:
host 120.79.77.160
port 3506
user root
password admin
```

## docker的使用

```bash
# 容器名称
sse_market_client_pc
sse_market_client_mobile
sse_market_manager
mysql
redis
sse_market_server
``

```bash
# 查看当前docker容器运行状态
docker ps -a

## 进入容器bash
docker exec -it <容器名称/容器ID> bash

# 停止容器
docker stop <容器名称/容器ID>

# 删除容器
docker rm <容器名称/容器ID>

# 查看镜像
docker images

# 删除镜像
docker rmi <镜像名称/镜像ID>

# 修改代码后如何部署
1. 自己电脑完成代码修改，在gitee进行pull request
2. 我来部署/你来部署
3. cd SSE_market_server
4. docker compose up --build -d <容器名称>
```