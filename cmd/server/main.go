package main

import (
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/Full-finger/NDisk/internal/auth"
	"github.com/Full-finger/NDisk/internal/config"
	"github.com/Full-finger/NDisk/internal/database"
	"github.com/Full-finger/NDisk/internal/file"
	"github.com/Full-finger/NDisk/internal/storage"
	"github.com/Full-finger/NDisk/internal/web"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// 初始化数据库
	db, err := database.New(cfg)
	if err != nil {
		log.Fatal("failed to connect database:", err)
	}

	// 自动迁移
	db.AutoMigrate(&auth.User{})

	// 初始化存储
	store := storage.NewLocal(cfg.Storage.Path)
	_ = store

	// 初始化认证模块
	authService := auth.NewService(db, cfg.JWTSecret)
	authHandler := auth.NewHandler(authService)

	// 初始化文件模块
	fileService := file.NewService(db, store)
	fileHandler := file.NewHandler(fileService)

	// 初始化 Web Handler
	webHandler := web.NewHandler(authService, fileService, cfg.JWTSecret)

	db.AutoMigrate(&auth.User{}, &file.File{})

	r := gin.Default()

	// 加载模板
	tmpl := template.Must(template.ParseGlob("web/templates/*.html"))
	r.SetHTMLTemplate(tmpl)

	// 静态文件服务
	r.Static("/static", "web/static")

	// 公开路由
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	// 页面路由（公开）
	r.GET("/login", webHandler.LoginPage)
	r.GET("/register", webHandler.RegisterPage)

	// 页面路由（需要认证）
	filesGroup := r.Group("/files")
	filesGroup.Use(webHandler.CookieAuthMiddleware())
	{
		filesGroup.GET("", webHandler.FilesPage)
	}

	// 创建速率限制器：每分钟最多 5 次登录/注册尝试
	authRateLimiter := auth.NewRateLimiter(5, time.Minute)

	// 认证路由（无需 JWT）
	authGroup := r.Group("/api/auth")
	{
		// 注册和登录接口应用速率限制
		authGroup.POST("/register", auth.RateLimitMiddleware(authRateLimiter), authHandler.Register)
		authGroup.POST("/login", auth.RateLimitMiddleware(authRateLimiter), authHandler.Login)
		authGroup.POST("/logout", authHandler.Logout)
	}

	// 受保护路由（需要 JWT）
	api := r.Group("/api")
	api.Use(auth.JWTMiddleware(cfg.JWTSecret))
	{
		api.GET("/me", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"user_id":  c.GetUint("user_id"),
				"username": c.GetString("username"),
			})
		})

		// 文件路由
		api.POST("/files/upload", fileHandler.Upload)
		api.GET("/files", fileHandler.List)
		api.POST("/folders", fileHandler.CreateFolder)
		api.GET("/files/:id/download", fileHandler.Download)
		api.HEAD("/files/:id/download", fileHandler.DownloadHead)
		api.DELETE("/files/:id", fileHandler.Delete)
		api.PUT("/files/:id/rename", fileHandler.Rename)
	}

	// 首页重定向到登录页
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/login")
	})

	log.Printf("Server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
