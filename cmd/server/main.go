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
	nfspkg "github.com/Full-finger/NDisk/internal/nfs"
	"github.com/Full-finger/NDisk/internal/share"
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
	db.AutoMigrate(&auth.User{}, &auth.RefreshToken{}, &file.File{}, &file.DownloadLink{}, &share.Share{}, &nfspkg.NFSToken{})

	store := storage.NewLocal(cfg.Storage.Path)

	authService := auth.NewService(db, cfg.JWTSecret)
	authHandler := auth.NewHandler(authService)

	fileService := file.NewService(db, store)
	zipCacheDir := filepath.Join(filepath.Dir(cfg.Storage.Path), "zip_cache")
	fileService.SetZipDir(zipCacheDir)
	fileService.StartCacheCleaner()
	fileHandler := file.NewHandler(fileService, cfg.JWTSecret)

	webHandler := web.NewHandler(authService, fileService, cfg.JWTSecret, cfg.Wallpaper.DefaultURL)

	shareService := share.NewService(db)
	shareHandler := share.NewHandler(shareService, fileService)

	// NFS 服务（如果启用）
	if cfg.NFS.Enabled {
		hmacKey := cfg.NFS.HMACKey
		if hmacKey == "" {
			hmacKey = cfg.JWTSecret // 回退使用 JWT 密钥
		}
		nfsServer := nfspkg.NewServer(fileService, store, hmacKey, cfg.NFS.Port)
		if err := nfsServer.Start(); err != nil {
			log.Printf("Warning: NFS server failed to start: %v", err)
		} else {
			log.Printf("NFS server enabled on :%s", cfg.NFS.Port)
		}
	}

	// NFS Token 管理 API
	nfsAPIHandler := nfspkg.NewAPIHandler(db)

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

	// 分享管理页面（需要认证 - refresh token cookie）
	shareGroup := r.Group("/share")
	shareGroup.Use(webHandler.CookieAuthMiddleware())
	{
		shareGroup.GET("", webHandler.SharesPage)
	}

	// NFS 管理页面（需要认证 - refresh token cookie）
	nfsGroup := r.Group("/nfs")
	nfsGroup.Use(webHandler.CookieAuthMiddleware())
	{
		nfsGroup.GET("", webHandler.NFSPage)
	}

	// 个人资料页面（需要认证 - refresh token cookie）
	profileGroup := r.Group("/profile")
	profileGroup.Use(webHandler.CookieAuthMiddleware())
	{
		profileGroup.GET("", webHandler.ProfilePage)
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

	// 分享页面（公开）
	r.GET("/s/:token", shareHandler.SharePage)

	// 分享 API（公开）
	r.POST("/api/shares/:token/verify", shareHandler.VerifyPassword)
	r.GET("/api/shares/:token/download", shareHandler.Download)

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

		// 分享（需要认证）
		api.POST("/shares", shareHandler.CreateShare)
		api.GET("/shares", shareHandler.ListShares)
		api.DELETE("/shares/:id", shareHandler.DeleteShare)

		// NFS Token 管理（需要认证）
		api.POST("/nfs/tokens", nfsAPIHandler.CreateToken)
		api.GET("/nfs/tokens", nfsAPIHandler.ListTokens)
		api.DELETE("/nfs/tokens/:id", nfsAPIHandler.DeleteToken)

		// 个人资料（需要认证）
		api.PUT("/auth/profile", authHandler.UpdateProfile)
		api.PUT("/auth/wallpaper", authHandler.UpdateWallpaper)
	}

	// 首页重定向
	r.GET("/", webHandler.IndexRedirect)

	log.Printf("Server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
