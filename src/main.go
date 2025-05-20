package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type Message struct {
	Name    string  `json:"name"`
	Amount  float32 `json:"amount"`
	Message string  `json:"message"`
}

type Config struct {
	Port            string
	FrontendURL     string
	AdminUsername   string
	AdminPassword   string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

func loadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	config := &Config{
		Port:            getEnvOrDefault("PORT", "8080"),
		FrontendURL:     getEnvOrDefault("FRONTEND_URL", "http://localhost:5173"),
		AdminUsername:   getEnvOrDefault("ADMIN_USERNAME", "admin"),
		AdminPassword:   os.Getenv("ADMIN_PASSWORD"),
		ReadTimeout:     time.Duration(getEnvIntOrDefault("READ_TIMEOUT", 5)) * time.Second,
		WriteTimeout:    time.Duration(getEnvIntOrDefault("WRITE_TIMEOUT", 10)) * time.Second,
		ShutdownTimeout: time.Duration(getEnvIntOrDefault("SHUTDOWN_TIMEOUT", 30)) * time.Second,
	}

	if config.AdminPassword == "" {
		return nil, fmt.Errorf("ADMIN_PASSWORD environment variable is required")
	}

	return config, nil
}

func setupRouter(config *Config) *gin.Engine {
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/ping"},
	}))

	// CORS middleware configuration
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{config.FrontendURL, "http://localhost:3000"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	corsConfig.AllowCredentials = true
	corsConfig.MaxAge = 12 * time.Hour

	r.Use(cors.New(corsConfig))

	// Health check endpoint
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong\n")
	})

	// WebSocket setup
	go hub.run()

	wss := r.Group("/ws")
	{
		wss.GET("/listen", listenHandler)
		wss.POST("/send", sendHandler) // Changed to POST as it's more appropriate for sending messages
	}

	// Authorized group
	authorized := r.Group("/", gin.BasicAuth(gin.Accounts{
		config.AdminUsername: config.AdminPassword,
	}))

	authorized.GET("messages", func(c *gin.Context) {
		user := c.MustGet(gin.AuthUserKey).(string)
		log.Printf("User %s accessed messages endpoint", user)

		from := c.DefaultQuery("from", time.Now().Add(-time.Hour).Format(time.RFC3339))
		to := c.DefaultQuery("to", time.Now().Format(time.RFC3339))

		fromTime, err := time.Parse(time.RFC3339, from)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 'from' parameter"})
			return
		}
		toTime, err := time.Parse(time.RFC3339, to)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 'to' parameter"})
			return
		}

		messages := getMessages(fromTime, toTime)
		c.JSON(http.StatusOK, gin.H{"messages": messages})
	})

	return r
}

func main() {
	// Load configuration
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	if err := initDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer dbPool.Close()

	// Setup router
	router := setupRouter(config)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      router,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server starting on port %s", config.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	// Close database connection
	closeDB()

	log.Println("Server exiting")
}

// Helper functions
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
