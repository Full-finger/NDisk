package main

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
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

	db, err := database.New(cfg)
	if err != nil {
		log.Fatal("failed to connect database:", err)
	}

	// 自动迁移
	db.AutoMigrate(&auth.User{}, &auth.RefreshToken{}, &file.File{}, &file.DownloadLink{})

	store := storage.NewLocal(cfg.Storage.Path)

	authService := auth.NewService(db, cfg.JWTSecret)
	authHandler := auth.NewHandler(authService)

	fileService := file.NewService(db, store)
	zipCacheDir := filepath.Join(filepath.Dir(cfg.Storage.Path), "zip_cache")
	fileService.SetZipDir(zipCacheDir)
	fileService.StartCacheCleaner()
	fileHandler := file.NewHandler(fileService, cfg.JWTSecret)

	webHandler := web.NewHandler(authService, fileService, cfg.JWTSecret)

	r := gin.Default()

	tmpl := template.Must(template.ParseGlob("web/templates/*.html"))
	r.SetHTMLTemplate(tmpl)

	r.Static("/static", "web/static")

	// 公开路由
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	// 页面路由（公开）
	r.GET("/login", webHandler.LoginPage)
	r.GET("/register", webHandler.RegisterPage)

	// 页面路由（需要认证 - refresh token cookie）
	filesGroup := r.Group("/files")
	filesGroup.Use(webHandler.CookieAuthMiddleware())
	{
		filesGroup.GET("", webHandler.FilesPage)
	}

	// 认证路由
	authRateLimiter := auth.NewRateLimiter(5, time.Minute)
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/register", auth.RateLimitMiddleware(authRateLimiter), authHandler.Register)
		authGroup.POST("/login", auth.RateLimitMiddleware(authRateLimiter), authHandler.Login)
		authGroup.POST("/refresh", authHandler.Refresh)
		authGroup.POST("/logout", authHandler.Logout)
	}

	// 下载短链接（无需认证）
	r.GET("/api/dl/:token", fileHandler.DownloadByToken)

	// 受保护路由（需要 access token JWT）
	api := r.Group("/api")
	api.Use(auth.JWTMiddleware(cfg.JWTSecret))
	{
		api.GET("/me", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"user_id":  c.GetUint("user_id"),
				"username": c.GetString("username"),
			})
		})

		api.POST("/files/upload", fileHandler.Upload)
		api.GET("/files/upload", fileHandler.TestChunk)
		api.GET("/files", fileHandler.List)
		api.POST("/folders", fileHandler.CreateFolder)
		api.DELETE("/files/:id", fileHandler.Delete)
		api.PUT("/files/:id/rename", fileHandler.Rename)
		api.PUT("/files/:id/move", fileHandler.Move)
		api.GET("/folders/all", fileHandler.ListAllFolders)

		// 下载链接生成（需要认证）
		api.POST("/files/:id/download-link", fileHandler.CreateDownloadLink)
		api.POST("/folders/:id/download-link", fileHandler.CreateFolderDownloadLink)
	}

	// 首页重定向
	r.GET("/", webHandler.IndexRedirect)

	log.Printf("Server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
