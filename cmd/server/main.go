package main

import (
	"log"
	"net/http"

	"github.com/Full-finger/NDisk/internal/auth"
	"github.com/Full-finger/NDisk/internal/config"
	"github.com/Full-finger/NDisk/internal/database"
	"github.com/Full-finger/NDisk/internal/storage"
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

	r := gin.Default()

	// 公开路由
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	// 认证路由（无需 JWT）
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
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
	}

	log.Printf("Server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
