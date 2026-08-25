package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/storage/journal"
	"heritage-tree-relocation-permit/internal/web"
	"heritage-tree-relocation-permit/internal/webassets"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(arguments []string) error {
	configuration, err := parseConfig(arguments)
	if err != nil {
		return err
	}
	dataDir := configuration.DataDir
	cleanup := func() {}
	if configuration.Selfcheck {
		dataDir, err = os.MkdirTemp("", "heritage-relocation-selfcheck-")
		if err != nil {
			return fmt.Errorf("创建自检数据目录失败: %w", err)
		}
		cleanup = func() { _ = os.RemoveAll(dataDir) }
	}
	defer cleanup()
	repository, err := journal.Open(dataDir)
	if err != nil {
		return fmt.Errorf("恢复日志仓库失败: %w", err)
	}
	defer repository.Close()
	service := application.NewService(repository, application.RealClock{}, nil)
	handler := web.NewHandler(service, webassets.NewHandler())
	listener, err := net.Listen("tcp", configuration.Address)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", configuration.Address, err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	actualAddress := listener.Addr().String()
	if configuration.Selfcheck {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		checkErr := runSelfcheck(ctx, "http://"+actualAddress)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		serveErr := <-serveErrors
		if checkErr != nil {
			return fmt.Errorf("自检失败: %w", checkErr)
		}
		if shutdownErr != nil {
			return fmt.Errorf("自检关闭服务失败: %w", shutdownErr)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		fmt.Printf("自检通过：真实监听 %s，完整流程已签发许可并验证时间线\n", actualAddress)
		return nil
	}
	log.Printf("古树迁移作业许可服务监听于 http://%s", actualAddress)
	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalContext.Done():
	case serveErr := <-serveErrors:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("优雅停止服务失败: %w", err)
	}
	serveErr := <-serveErrors
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return repository.Close()
}
