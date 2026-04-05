package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/Full-finger/NDisk/internal/config"
	"github.com/Full-finger/NDisk/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// 初始化存储
	store := storage.NewLocal(cfg.Storage.Path)
	_ = store  // 临时：先使用一下，避免报错。明天会真正用到

	r := gin.Default()

	// 健康检查
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	log.Printf("Server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}