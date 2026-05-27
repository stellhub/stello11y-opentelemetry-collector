# 单机部署说明

本文档说明如何在 Linux 单机上基于源码构建并通过 `systemd` 部署 `stello11y-opentelemetry-collector`。

## 1. 环境要求

- Linux 主机
- Go 版本以 `go.mod` 为准
- 机器可以访问：
  - Stellflow: `stellflow://127.0.0.1:9092` 或实际地址
  - VictoriaMetrics: `http://127.0.0.1:8428/opentelemetry/v1/metrics` 或实际地址
  - Tempo: `192.168.1.14:4317` 或实际地址

## 2. 拉取源码

```bash
cd /data
git clone https://github.com/stellhub/stello11y-opentelemetry-collector.git
cd /data/stello11y-opentelemetry-collector
```

如需使用指定分支：

```bash
git checkout <branch-name>
```

## 3. 构建二进制

当前项目入口位于 `./cmd`，构建命令如下：

```bash
go mod download
go build -o ./bin/stello11y-opentelemetry-collector ./cmd
```

验证二进制：

```bash
./bin/stello11y-opentelemetry-collector --version
./bin/stello11y-opentelemetry-collector validate --config ./configs/collector.yaml
```

## 4. 安装目录

推荐安装目录：

```bash
sudo mkdir -p /opt/stello11y-opentelemetry-collector/bin
sudo mkdir -p /etc/stello11y-opentelemetry-collector
sudo mkdir -p /var/log/stello11y-opentelemetry-collector
```

复制二进制和配置：

```bash
sudo cp ./bin/stello11y-opentelemetry-collector /opt/stello11y-opentelemetry-collector/bin/
sudo cp ./configs/collector.yaml /etc/stello11y-opentelemetry-collector/collector.yaml
```

确认配置：

```bash
sudo /opt/stello11y-opentelemetry-collector/bin/stello11y-opentelemetry-collector validate \
  --config /etc/stello11y-opentelemetry-collector/collector.yaml
```

## 5. systemd 服务

创建服务文件：

```bash
sudo tee /etc/systemd/system/stello11y-opentelemetry-collector.service >/dev/null <<'EOF'
[Unit]
Description=stello11y OpenTelemetry Collector
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/opt/stello11y-opentelemetry-collector/bin/stello11y-opentelemetry-collector --config /etc/stello11y-opentelemetry-collector/collector.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
```

加载 systemd 配置：

```bash
sudo systemctl daemon-reload
```

### 5.1 从 go build 到 systemd start

源码更新后，可以按下面顺序完成构建、安装、配置校验和启动：

```bash
cd /data/stello11y-opentelemetry-collector
go mod download
go build -o ./bin/stello11y-opentelemetry-collector ./cmd

sudo install -D -m 0755 ./bin/stello11y-opentelemetry-collector \
  /opt/stello11y-opentelemetry-collector/bin/stello11y-opentelemetry-collector
sudo install -D -m 0644 ./configs/collector.yaml \
  /etc/stello11y-opentelemetry-collector/collector.yaml

sudo /opt/stello11y-opentelemetry-collector/bin/stello11y-opentelemetry-collector validate \
  --config /etc/stello11y-opentelemetry-collector/collector.yaml

sudo systemctl daemon-reload
sudo systemctl enable stello11y-opentelemetry-collector
sudo systemctl start stello11y-opentelemetry-collector
sudo systemctl status stello11y-opentelemetry-collector
```

如果只是替换二进制，推荐先构建和校验，再重启服务：

```bash
cd /data/stello11y-opentelemetry-collector
go build -o ./bin/stello11y-opentelemetry-collector ./cmd
sudo install -D -m 0755 ./bin/stello11y-opentelemetry-collector \
  /opt/stello11y-opentelemetry-collector/bin/stello11y-opentelemetry-collector
sudo /opt/stello11y-opentelemetry-collector/bin/stello11y-opentelemetry-collector validate \
  --config /etc/stello11y-opentelemetry-collector/collector.yaml
sudo systemctl restart stello11y-opentelemetry-collector
sudo systemctl status stello11y-opentelemetry-collector
```

### 5.2 systemd 控制指南

设置开机启动：

```bash
sudo systemctl enable stello11y-opentelemetry-collector
```

启动服务：

```bash
sudo systemctl start stello11y-opentelemetry-collector
```

查看状态：

```bash
sudo systemctl status stello11y-opentelemetry-collector
```

查看日志：

```bash
sudo journalctl -u stello11y-opentelemetry-collector -f
```

重启服务：

```bash
sudo systemctl restart stello11y-opentelemetry-collector
```

停止服务：

```bash
sudo systemctl stop stello11y-opentelemetry-collector
```

查看最近 200 行日志：

```bash
sudo journalctl -u stello11y-opentelemetry-collector -n 200 --no-pager
```

查看当前 systemd unit 内容：

```bash
sudo systemctl cat stello11y-opentelemetry-collector
```

检查服务是否处于运行和开机自启状态：

```bash
systemctl is-active stello11y-opentelemetry-collector
systemctl is-enabled stello11y-opentelemetry-collector
```

修改 service 文件后重新加载：

```bash
sudo systemctl daemon-reload
sudo systemctl restart stello11y-opentelemetry-collector
```

清理失败状态后再启动：

```bash
sudo systemctl reset-failed stello11y-opentelemetry-collector
sudo systemctl start stello11y-opentelemetry-collector
```

## 6. 配置检查

默认配置文件：

```text
/etc/stello11y-opentelemetry-collector/collector.yaml
```

当前 Collector 默认监听：

```text
localhost:4317
```

确认端口监听：

```bash
sudo ss -lntp | grep 4317
```

确认配置中后端地址：

```bash
grep -n "stellflow\\|victoriametrics\\|tempo\\|endpoint" /etc/stello11y-opentelemetry-collector/collector.yaml
```

## 7. 升级流程

```bash
cd /data/stello11y-opentelemetry-collector
git pull
go mod download
go build -o ./bin/stello11y-opentelemetry-collector ./cmd
sudo systemctl stop stello11y-opentelemetry-collector
sudo cp ./bin/stello11y-opentelemetry-collector /opt/stello11y-opentelemetry-collector/bin/
sudo /opt/stello11y-opentelemetry-collector/bin/stello11y-opentelemetry-collector validate \
  --config /etc/stello11y-opentelemetry-collector/collector.yaml
sudo systemctl start stello11y-opentelemetry-collector
sudo systemctl status stello11y-opentelemetry-collector
```

## 8. 卸载

```bash
sudo systemctl stop stello11y-opentelemetry-collector
sudo systemctl disable stello11y-opentelemetry-collector
sudo rm -f /etc/systemd/system/stello11y-opentelemetry-collector.service
sudo systemctl daemon-reload
sudo rm -rf /opt/stello11y-opentelemetry-collector
sudo rm -rf /etc/stello11y-opentelemetry-collector
```
