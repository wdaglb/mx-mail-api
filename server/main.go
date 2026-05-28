package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"mx-mail-api/internal/api"
	"mx-mail-api/internal/config"
	"mx-mail-api/internal/repository"
	"mx-mail-api/internal/service"
	"mx-mail-api/internal/smtpserver"
	"mx-mail-api/internal/storage"

	"github.com/gin-gonic/gin"
)

const webDistPath = "web/dist"

/**
 * main 初始化 Gin HTTP 服务和 SMTP/MX TCP 服务。
 *
 * 参数：无。
 * 返回值：无。
 * 失败条件：当 Postgres 初始化失败、任一监听端口不可用，或服务异常退出时，进程会 panic。
 */
func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	store, err := storage.OpenPostgresStore(cfg.Database.DSN)
	if err != nil {
		panic(err)
	}
	userRepo := repository.NewUserRepository(store.DB())
	domainRepo := repository.NewDomainRepository(store.DB())
	messageRepo := repository.NewMessageRepository(store.DB())
	temporaryMailboxRepo := repository.NewTemporaryMailboxRepository(store.DB())
	userService := service.NewUserService(userRepo, cfg)
	domainService := service.NewDomainService(domainRepo, cfg)
	temporaryMailboxService := service.NewTemporaryMailboxService(temporaryMailboxRepo, domainRepo)
	messageService := service.NewMessageService(messageRepo, domainRepo, temporaryMailboxService)

	apiServer := api.New(userService, domainService, messageService, temporaryMailboxService, cfg)
	if err := userService.BootstrapInitialAdmin(context.Background()); err != nil {
		panic(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 2)

	smtp := &smtpserver.Server{
		Addr:     cfg.SMTP.Addr,
		Hostname: cfg.SMTP.Hostname,
		Messages: messageService,
	}
	go func() {
		log.Printf("smtp service starting addr=%q hostname=%q", cfg.SMTP.Addr, cfg.SMTP.Hostname)
		errs <- smtp.ListenAndServe(ctx)
	}()

	httpServer := &http.Server{
		Addr:    cfg.HTTP.Addr,
		Handler: setupRouter(apiServer),
	}
	go func() {
		log.Printf("http service starting addr=%q", cfg.HTTP.Addr)
		// Gin 的 Router.Run 不方便外部优雅停止；这里显式使用 http.Server 以便收到系统信号后关闭。
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errs <- err
			return
		}

		errs <- nil
	}()

	select {
	case err := <-errs:
		if err != nil {
			panic(err)
		}
	case <-ctx.Done():
	}

	if err := httpServer.Shutdown(context.Background()); err != nil {
		panic(err)
	}
}

/**
 * setupRouter 构建服务的全部 HTTP 路由。
 *
 * 参数：apiServer 为已经初始化的 API 服务实例。
 * 返回值：可由 main 启动或由测试直接调用的 Gin 引擎。
 * 失败条件：当前没有失败分支；路由注册是确定性的，并且不依赖外部资源。
 */
func setupRouter(apiServer *api.Server) *gin.Engine {
	router := gin.Default()

	// 健康检查保持无外部依赖，便于容器探针和本地 smoke test 快速判断进程是否存活。
	router.GET("/health", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})
	apiServer.RegisterRoutes(router)
	registerWebRoutes(router)

	return router
}

/**
 * registerWebRoutes 注册前端静态资源和单页应用入口。
 *
 * 参数：
 * - router：Gin 路由。
 * 返回值：无。
 * 失败条件：无；前端构建产物不存在时跳过静态资源注册，方便后端测试和本地仅 API 启动。
 */
func registerWebRoutes(router *gin.Engine) {
	if _, err := os.Stat(filepath.Join(webDistPath, "index.html")); err != nil {
		return
	}

	router.Static("/static", filepath.Join(webDistPath, "static"))
	router.StaticFile("/favicon.png", filepath.Join(webDistPath, "favicon.png"))
	router.NoRoute(func(ctx *gin.Context) {
		// API 和文档原文路径不应回退到前端页面，避免调用方把 404 误判为页面 HTML。
		path := ctx.Request.URL.Path
		if path == "/docs/api.md" || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/openapi/") {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "not_found",
					"message": "route not found",
				},
			})
			return
		}

		ctx.File(filepath.Join(webDistPath, "index.html"))
	})
}
