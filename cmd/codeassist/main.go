package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/txarli-san/codeassist/internal/llm"
	"github.com/txarli-san/codeassist/internal/scanner"
	"github.com/txarli-san/codeassist/internal/storage"
)

func main() {
	// Define global flags
	dbPath := flag.String("db", "codeassist.db", "Path to the SQLite database file")
	ollamaURL := flag.String("ollama-url", "http://localhost:11434", "URL for Ollama API")
	ollamaModel := flag.String("model", "codellama", "Ollama model to use")

	// Check if command provided
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Find the subcommand position
	cmdIndex := 1
	for i := 1; i < len(os.Args); i++ {
		if !strings.HasPrefix(os.Args[i], "-") {
			cmdIndex = i
			break
		}
	}

	// Parse global flags (everything before the subcommand)
	flag.CommandLine.Parse(os.Args[1:cmdIndex])

	// Initialize database
	db, err := storage.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize Ollama client
	ollamaClient := llm.NewOllamaClient(*ollamaURL, *ollamaModel)

	// Get the subcommand
	cmd := os.Args[cmdIndex]
	cmdArgs := os.Args[cmdIndex+1:]

	// Handle based on subcommand
	switch cmd {
	case "scan":
		scanCmd := flag.NewFlagSet("scan", flag.ExitOnError)
		scanPath := scanCmd.String("path", ".", "Path to scan for code")
		scanLangs := scanCmd.String("langs", "go,rb,js", "Comma-separated list of languages to scan")
		scanCmd.Parse(cmdArgs)

		fmt.Printf("Scanning directory: %s for languages: %s\n", *scanPath, *scanLangs)
		if err := scanner.ScanDirectory(*scanPath, *scanLangs, db); err != nil {
			log.Fatalf("Scan failed: %v", err)
		}

	case "query":
		queryCmd := flag.NewFlagSet("query", flag.ExitOnError)
		queryStr := queryCmd.String("q", "", "Question to ask")
		queryCmd.Parse(cmdArgs)

		if *queryStr == "" {
			fmt.Println("Please provide a query with -q")
			os.Exit(1)
		}

		err = processQuery(*queryStr, db, ollamaClient)
		if err != nil {
			log.Fatalf("Query failed: %v", err)
		}

	case "interactive":
		runInteractiveMode(db, ollamaClient)

	case "stats":
		showStats(db)

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  codeassist [global options] command [command options]")
	fmt.Println("\nGlobal options:")
	fmt.Println("  -db string       Path to the SQLite database file (default \"codeassist.db\")")
	fmt.Println("  -ollama-url string  URL for Ollama API (default \"http://localhost:11434\")")
	fmt.Println("  -model string    Ollama model to use (default \"codellama\")")
	fmt.Println("\nCommands:")
	fmt.Println("  scan         Scan a directory for code")
	fmt.Println("    -path string    Path to scan (default \".\")")
	fmt.Println("    -langs string   Languages to scan (default \"go,rb,js\")")
	fmt.Println("  query        Ask a question about your codebase")
	fmt.Println("    -q string       Question to ask")
	fmt.Println("  interactive  Start interactive mode")
	fmt.Println("  stats        Show database statistics")
}

func processQuery(query string, db *storage.DB, client *llm.OllamaClient) error {
	fmt.Println("Finding relevant code...")

	// Try to use the streaming response with whole files
	files, err := db.FindRelevantFiles(query, 5)
	if err == nil && len(files) > 0 {
		fmt.Printf("Found %d relevant files. Using file-based streaming response...\n", len(files))
		return client.GenerateStreamingResponse(query, db)
	}

	// Fall back to the entity-based approach if no files found or error occurred
	entities, err := db.FindRelevantEntities(query)
	if err != nil {
		return fmt.Errorf("failed to find relevant code: %v", err)
	}

	fmt.Printf("Found %d relevant code entities. Using entity-based response...\n", len(entities))

	response, err := client.GenerateResponse(query, entities)
	if err != nil {
		return err
	}

	fmt.Println("\nResponse:")
	fmt.Println(response)
	return nil
}

func runInteractiveMode(db *storage.DB, client *llm.OllamaClient) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== CodeAssist Interactive Mode ===")
	fmt.Println("Type your questions about the codebase. Type 'exit' to quit.")

	for {
		fmt.Print("\n> ")
		query, _ := reader.ReadString('\n')
		query = strings.TrimSpace(query)

		if query == "exit" || query == "quit" {
			break
		}

		if query == "" {
			continue
		}

		err := processQuery(query, db, client)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	fmt.Println("Goodbye!")
}

func showStats(db *storage.DB) {
	stats, err := db.GetStats()
	if err != nil {
		fmt.Printf("Error getting stats: %v\n", err)
		return
	}

	fmt.Println("=== Database Statistics ===")
	fmt.Printf("Total files: %d\n", stats["files"])
	fmt.Printf("Total entities: %d\n", stats["entities"])

	fmt.Println("\nEntity types:")
	for k, v := range stats {
		if strings.HasPrefix(k, "type_") {
			entityType := strings.TrimPrefix(k, "type_")
			fmt.Printf("  %s: %d\n", entityType, v)
		}
	}

	fmt.Println("\nLanguages:")
	for k, v := range stats {
		if strings.HasPrefix(k, "lang_") {
			lang := strings.TrimPrefix(k, "lang_")
			fmt.Printf("  %s: %d\n", lang, v)
		}
	}
}
