package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

type config struct {
	Address   string
	DataDir   string
	Selfcheck bool
}

func parseConfig(arguments []string) (config, error) {
	address := defaultAddress
	if portText := strings.TrimSpace(os.Getenv("PORT")); portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return config{}, fmt.Errorf("PORT 必须是 1 至 65535 的端口号")
		}
		address = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	set := flag.NewFlagSet("heritage-tree-relocation-permit", flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	set.StringVar(&address, "addr", address, "HTTP 监听地址，仅允许回环地址")
	dataDir := "data"
	selfcheck := false
	set.StringVar(&dataDir, "data-dir", dataDir, "事件日志与投影快照目录")
	set.BoolVar(&selfcheck, "selfcheck", false, "通过真实 HTTP 监听运行完整冒烟流程后退出")
	if err := set.Parse(arguments); err != nil {
		return config{}, err
	}
	if set.NArg() != 0 {
		return config{}, errors.New("存在无法识别的位置参数")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return config{}, fmt.Errorf("addr 必须为 host:port: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return config{}, errors.New("addr 必须使用 127.0.0.1 或其他明确的回环 IP")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return config{}, errors.New("addr 端口必须在 0 至 65535 之间")
	}
	if strings.TrimSpace(dataDir) == "" {
		return config{}, errors.New("data-dir 不能为空")
	}
	return config{Address: address, DataDir: dataDir, Selfcheck: selfcheck}, nil
}
