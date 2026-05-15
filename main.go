package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	dbPath := getEnv("MEM_DB_PATH", ".memory/memory.db")
	readonly := isTruthy(getEnv("MEM_READONLY", "false"))
	projectId := os.Getenv("MEM_PROJECT_ID")

	db, err := openMemoryDb(dbPath)
	if err != nil {
		log.Fatalf("Fatal: %v", err)
	}
	defer db.Close()

	store := newMemoryStore(db, struct {
		readonly  bool
		projectId string
	}{readonly: readonly, projectId: projectId})

	server := mcp.NewServer(&mcp.Implementation{Name: "mem", Version: "0.1.0"}, nil)
	registerTools(server, store)

	log.Println("mem running on stdio")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func getEnv(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func isTruthy(val string) bool {
	lower := strings.ToLower(val)
	return lower == "1" || lower == "true" || lower == "yes"
}
