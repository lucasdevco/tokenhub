package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lucasdevco/tokenhub/internal/management"
	"github.com/lucasdevco/tokenhub/internal/service"
)

func main() {
	config := flag.String("config", "config.yaml", "configuration file")
	reindex := flag.Bool("reindex-logs", false, "rebuild the log index without starting the proxy")
	address := flag.String("listen", "127.0.0.1:5602", "loopback management address")
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	if flag.NArg() == 1 {
		*config = flag.Arg(0)
	}
	if *reindex {
		if err := reindexLogs(*config); err != nil {
			log.Fatal(err)
		}
		return
	}

	programLog, err := service.OpenProgramLog(*config)
	if err != nil {
		log.Printf("无法保存程序日志: %v", err)
	} else {
		defer programLog.Close()
	}
	manager, err := service.NewManager(*config)
	if err != nil {
		log.Fatal(err)
	}
	server, err := management.New(manager, management.Options{Address: *address})
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()
	defer func() {
		if err := manager.Stop(); err != nil {
			log.Printf("停止代理或日志收尾失败: %v", err)
		}
	}()

	if err := manager.Start(); err != nil {
		log.Printf("代理启动失败，可在设置页修正配置后重新启动: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	errors := make(chan error, 1)
	go func() { errors <- server.Serve() }()

	if status := manager.Status(); status.State == "running" {
		log.Printf("代理地址: %s", status.Address)
	}
	log.Printf("设置页面: %s/settings", server.URL)
	log.Printf("请求日志: %s/logs/", server.URL)
	log.Print("按 Ctrl+C 退出")

	select {
	case <-ctx.Done():
	case err := <-errors:
		if err != nil {
			log.Print(err)
		}
	}
}

func reindexLogs(filename string) error {
	cfg, err := service.LoadConfig(filename)
	if err != nil {
		return err
	}
	store, err := management.NewStore(cfg.Logging.LogDir, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := store.Reindex(ctx); err != nil {
		return err
	}
	log.Print("日志索引已更新")
	return nil
}
