package main

import (
	"database/sql"
	"flag"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

func main() {
	// Define a command-line flag for the database file path
	dbPath := flag.String("db", "./gokv.db", "path to the SQLite database file")
	flag.Parse()

	// Initialize the database
	db, err := setupDatabase(*dbPath)
	if err != nil {
		log.Fatalf("Failed to set up database: %v", err)
	}
	defer db.Close()

	// Set up Gin router
	router := gin.Default()

	// Endpoint to create a new bucket and token
	router.POST("/bucket", createBucketHandler(db))

	// Create a group for authenticated routes
	// The middleware now needs the DB connection to validate tokens
	api := router.Group("/kv", authMiddleware(db))

	// Define API endpoints
	api.GET("/:key", getHandler(db))
	api.POST("/:key", putHandler(db))
	api.DELETE("/:key", deleteHandler(db))

	// Start the server
	log.Println("Starting gokv server on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// setupDatabase initializes the SQLite database and creates the necessary table.
func setupDatabase(dbFile string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return nil, err
	}

	// SQL statements to create tables
	createKVSQL := `CREATE TABLE IF NOT EXISTS kv_store (
        "bucket" TEXT NOT NULL,
        "key" TEXT NOT NULL,
        "value" TEXT,
        PRIMARY KEY (bucket, key)
    );`

	createBucketsSQL := `CREATE TABLE IF NOT EXISTS buckets (
		"bucket_id" TEXT PRIMARY KEY,
		"email" TEXT NOT NULL UNIQUE,
		"token" TEXT NOT NULL UNIQUE
	);`

	// Execute creation statements
	if _, err := db.Exec(createKVSQL); err != nil {
		return nil, err
	}
	if _, err := db.Exec(createBucketsSQL); err != nil {
		return nil, err
	}

	log.Printf("Database initialized and table created at %s", dbFile)
	return db, nil
}

// authMiddleware handles token-based authentication.
func authMiddleware(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header format must be Bearer {token}"})
			c.Abort()
			return
		}

		token := parts[1]
		var bucketID string
		query := "SELECT bucket_id FROM buckets WHERE token = ?"
		err := db.QueryRow(query, token).Scan(&bucketID)

		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
				c.Abort()
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error during authentication"})
			log.Printf("Error authenticating token: %v", err)
			c.Abort()
			return
		}

		// Store the bucket in the context for handlers to use
		c.Set("bucket", bucketID)
		c.Next()
	}
}

// createBucketRequest defines the structure for the /bucket endpoint request body.
type createBucketRequest struct {
	Email string `json:"email" binding:"required"`
}

// createBucketHandler creates a new bucket, generates a token, and returns them.
func createBucketHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createBucketRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
			return
		}

		bucketID := uuid.New().String()
		token := uuid.NewString()

		query := "INSERT INTO buckets (bucket_id, email, token) VALUES (?, ?, ?)"
		_, err := db.Exec(query, bucketID, req.Email, token)
		if err != nil {
			// Use strings.Contains for broad compatibility with SQLite error messages
			if strings.Contains(err.Error(), "UNIQUE constraint failed: buckets.email") {
				c.JSON(http.StatusConflict, gin.H{"error": "Email address already in use"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create bucket"})
			log.Printf("Error creating bucket for email '%s': %v", req.Email, err)
			return
		}

		c.JSON(http.StatusCreated, gin.H{"bucket_id": bucketID, "token": token})
	}
}

// getHandler retrieves a value for a given key.
func getHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		bucket := c.GetString("bucket")
		key := c.Param("key")

		var value string
		query := "SELECT value FROM kv_store WHERE bucket = ? AND key = ?"
		err := db.QueryRow(query, bucket, key).Scan(&value)

		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"error": "Key not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			log.Printf("Error getting key '%s' from bucket '%s': %v", key, bucket, err)
			return
		}

		c.String(http.StatusOK, value)
	}
}

// putHandler creates or updates a key-value pair.
func putHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		bucket := c.GetString("bucket")
		key := c.Param("key")

		value, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read request body"})
			return
		}

		// Using INSERT OR REPLACE to handle both creation and updates (UPSERT).
		// In SQLite, this is an efficient way to perform an upsert.
		query := "INSERT OR REPLACE INTO kv_store (bucket, key, value) VALUES (?, ?, ?)"
		_, err = db.Exec(query, bucket, key, string(value))

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			log.Printf("Error putting key '%s' in bucket '%s': %v", key, bucket, err)
			return
		}

		c.Status(http.StatusCreated)
	}
}

// deleteHandler removes a key-value pair.
func deleteHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		bucket := c.GetString("bucket")
		key := c.Param("key")

		query := "DELETE FROM kv_store WHERE bucket = ? AND key = ?"
		result, err := db.Exec(query, bucket, key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			log.Printf("Error deleting key '%s' from bucket '%s': %v", key, bucket, err)
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			log.Printf("Error getting rows affected for key '%s' in bucket '%s': %v", key, bucket, err)
			return
		}

		if rowsAffected == 0 {
			// While DELETE is idempotent, you might want to know if the key existed.
			// Returning 404 here is a valid choice, but 204 is also common.
			c.JSON(http.StatusNotFound, gin.H{"error": "Key not found"})
			return
		}

		c.Status(http.StatusNoContent)
	}
}
