# kidtok · qimeng-feed

抖音式全屏沉浸短视频 Feed，内容来自自建网盘（AList）。后端 Go 单二进制，前端原生 HTML/CSS/JS 全部 embed 进可执行文件，部署只有一个容器。

- 全屏上下滑动、scroll-snap 逐屏切换，移动端手势 + 桌面滚轮都可用
- 双击点赞动画、右侧互动栏、合集连播、下拉刷新
- 视频走同源代理 `/api/video/stream`，由服务端实时换取网盘直链，浏览器不会触发 ORB/跨域拦截
- 手机端自动播放策略适配：静音起播，首次触摸后恢复声音；默认静音可在「设置」里改
- 网盘配置可在前端「我的 → 服务器配置」里改，落盘到 `config.json`

## 快速开始（Docker）

```bash
docker run -d --name kidtok \
  -p 8080:8080 \
  -v $(pwd)/data:/data \
  -e QIMENG_FANTAIYING_URL="http://饭太硬地址" \
  -e QIMENG_ALIST_URL="http://192.168.0.10:5244" \
  -e QIMENG_ALIST_USERNAME="admin" \
  -e QIMENG_ALIST_PASSWORD="你的密码" \
  -e TZ=Asia/Shanghai \
  --restart unless-stopped \
  burukkux/kidtok:latest
```

浏览器打开 `http://<主机IP>:8080` 即可。也可以先只起容器，再在前端「我的 → 服务器配置」里填网盘地址和账号。

镜像支持 `linux/amd64` 与 `linux/arm64`（群晖/威联通/树莓派都能直接跑）。

> 容器以非 root 用户（uid `10001`）运行，`/data` 需要可写。若日志里出现 `permission denied`，先执行 `sudo chown -R 10001:10001 data`；或者临时在 compose 里加 `user: "0:0"` 以 root 运行。

## Docker Compose

仓库根目录已附带 [`docker-compose.yml`](docker-compose.yml)，按需改环境变量后：

```bash
docker compose up -d      # 启动
docker compose logs -f    # 看日志
docker compose pull && docker compose up -d   # 更新到最新镜像
```

关键点：

- `environment` 里的 `QIMENG_*` 直接覆盖配置，优先级最高（高于 `config.yaml`）
- `./data` 挂到容器 `/data`，里面存的是 `config.json`（前端配置页保存的网盘参数），删掉容器不会丢配置
- 端口只映射 `8080`，容器内监听地址由 `QIMENG_SERVER_ADDR` 决定，默认 `:8080`

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `QIMENG_SERVER_ADDR` | `:8080` | HTTP 监听地址 |
| `QIMENG_FANTAIYING_URL` | 空 | 饭太硬影视源接口，未配置时接口返回配置缺失错误 |
| `QIMENG_USER_AGENT` | `okhttp/3.15` | 请求上游影视源用的 UA |
| `QIMENG_SOURCE_TIMEOUT` | `15s` | 上游影视源超时，支持 `15s` 或纯数字（秒） |
| `QIMENG_ALIST_URL` | 空 | AList 地址，如 `http://192.168.0.10:5244` |
| `QIMENG_ALIST_USERNAME` | 空 | AList 用户名 |
| `QIMENG_ALIST_PASSWORD` | 空 | AList 密码（建议用只读账号） |
| `QIMENG_ALIST_TOKEN` | 空 | 已有 token，填了就不再走登录 |
| `QIMENG_ALIST_TIMEOUT` | `15s` | AList 请求超时 |
| `QIMENG_CONFIG` | `config.yaml` | YAML 配置文件路径，文件不存在时只用环境变量 |
| `QIMENG_CONFIG_JSON` | `config.json` | 前端保存的网盘配置落盘路径，镜像内默认 `/data/config.json` |

优先级：**环境变量 > `config.yaml` > 内置默认值**。

## 本地运行（不用 Docker）

需要 Go 1.22+：

```bash
cp config.yaml.example config.yaml   # 然后按需改里面的地址和账号
go run ./cmd/server
```

前端资源是 `embed` 进二进制的：改完 `web/` 下的文件必须重启进程，浏览器刷新才看得到新内容。

## 构建镜像

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t burukkux/kidtok:latest -t burukkux/kidtok:1.0.0 --push .
```

国内网络直连 `auth.docker.io` 经常超时，可在 Docker Desktop 里配置代理，或改用镜像源加速后重试。
