package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	dbPool *pgxpool.Pool
	// SQL queries as constants to avoid string concatenation and improve maintainability
	insertMessageQuery = `
		INSERT INTO tts_messages (session_id, name, amount, message, description) 
		VALUES ($1, $2, $3, $4, $5)
	`
	selectMessagesQuery = `
		SELECT name, amount, message, description, created_at 
		FROM tts_messages 
		WHERE created_at >= $1 AND created_at <= $2 
		ORDER BY created_at DESC
	`
)

// DBConfig holds database configuration
type DBConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// loadDBConfig loads database configuration from environment variables
func loadDBConfig() (*DBConfig, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}

	return &DBConfig{
		URL:             url,
		MaxConns:        int32(getEnvIntOrDefault("DB_MAX_CONNS", 25)),
		MinConns:        int32(getEnvIntOrDefault("DB_MIN_CONNS", 5)),
		MaxConnLifetime: time.Duration(getEnvIntOrDefault("DB_MAX_CONN_LIFETIME", 3600)) * time.Second,
		MaxConnIdleTime: time.Duration(getEnvIntOrDefault("DB_MAX_CONN_IDLE_TIME", 1800)) * time.Second,
	}, nil
}

// Check if Row with given session ID exists
func checkSessionID(sessionID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	err := dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM tts_messages WHERE session_id = $1", sessionID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to query database: %w", err)
	}

	log.Printf("Session ID %s exists check: count = %d", sessionID, count)

	return count > 0, nil
}

func initDB() error {
	config, err := loadDBConfig()
	if err != nil {
		return fmt.Errorf("failed to load database config: %w", err)
	}

	poolConfig, err := pgxpool.ParseConfig(config.URL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Configure connection pool
	poolConfig.MaxConns = config.MaxConns
	poolConfig.MinConns = config.MinConns
	poolConfig.MaxConnLifetime = config.MaxConnLifetime
	poolConfig.MaxConnIdleTime = config.MaxConnIdleTime

	// Disable statement caching to avoid prepared statement conflicts
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	// Create connection pool
	dbPool, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := dbPool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	log.Printf("Successfully connected to database with pool size: %d", config.MaxConns)
	return nil
}

// addMessage adds a new message to the database
func addMessage(sessionID string, name string, amount float32, message string, description string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := dbPool.Exec(ctx, insertMessageQuery,
		sessionID,
		name,
		amount,
		message,
		description,
	)
	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	return nil
}

// getMessages retrieves messages from the database within the specified time range
func getMessages(from time.Time, to time.Time) []Message {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := dbPool.Query(ctx, selectMessagesQuery, from, to)
	if err != nil {
		log.Printf("Error querying database: %v", err)
		return []Message{}
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var createdAt time.Time
		if err := rows.Scan(&msg.Name, &msg.Amount, &msg.Message, &msg.Description, &createdAt); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}
		messages = append(messages, msg)
	}

	if err = rows.Err(); err != nil {
		log.Printf("Error iterating rows: %v", err)
	}

	return messages
}

// closeDB closes the database connection pool
func closeDB() {
	if dbPool != nil {
		dbPool.Close()
		log.Println("Database connection pool closed")
	}
}
