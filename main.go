package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/jgalec/mem/runtime"
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

	rt := runtime.New(runtime.DefaultConfig())
	defer rt.Shutdown()

	store := newMemoryStore(db, struct {
		readonly  bool
		projectId string
		rt        *runtime.Runtime
	}{readonly: readonly, projectId: projectId, rt: rt})

	rt.SetOnFlush(func(ops []runtime.WriteOp) error {
		for _, op := range ops {
			if _, err := db.Exec(op.Query, op.Args...); err != nil {
				return err
			}
		}
		return nil
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "mem", Version: "0.2.0"}, nil)
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
