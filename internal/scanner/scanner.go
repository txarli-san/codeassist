package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/txarli-san/codeassist/internal/scanner/parsers"
	"github.com/txarli-san/codeassist/internal/storage"
)

// ScanDirectory recursively scans a directory for code files
func ScanDirectory(rootDir, languages string, db *storage.DB) error {
	// Parse languages list
	langs := strings.Split(languages, ",")
	langMap := make(map[string]bool)
	for _, l := range langs {
		langMap[strings.TrimSpace(l)] = true
	}

	fmt.Printf("Starting scan of directory: %s\n", rootDir)
	fmt.Printf("Languages to scan: %s\n", languages)

	// Count statistics
	var stats struct {
		filesScanned  int
		entitiesFound int
		errors        int
	}

	// Walk directory
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("Error accessing path %s: %v\n", path, err)
			stats.errors++
			return filepath.SkipDir
		}

		// Skip directories we don't want to scan
		if info.IsDir() {
			basename := filepath.Base(path)
			if basename == ".git" || basename == "node_modules" || basename == "vendor" || basename == ".bundle" {
				fmt.Printf("Skipping directory: %s\n", path)
				return filepath.SkipDir
			}
			return nil
		}

		// Get file extension
		ext := strings.TrimPrefix(filepath.Ext(path), ".")

		// Map some extensions to their language
		switch ext {
		case "jsx", "ts", "tsx":
			ext = "js" // Treat as JavaScript
		case "rake", "gemspec":
			ext = "rb" // Treat as Ruby
		case "go":
			ext = "go" // Already correct, just for clarity
		}

		// Check if file should be scanned based on extension
		if !langMap[ext] {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("Warning: Could not read file %s: %v\n", path, err)
			stats.errors++
			return nil
		}

		fmt.Printf("Scanning file: %s\n", path)
		stats.filesScanned++

		// Store the whole file
		if err := db.StoreWholeFile(path, ext, info.ModTime().Unix(), string(content)); err != nil {
			fmt.Printf("Warning: Could not store whole file %s: %v\n", path, err)
			stats.errors++
			// Continue anyway - don't return
		}

		// Parse file based on language
		var parser parsers.Parser
		switch ext {
		case "go":
			parser = &parsers.GoParser{}
		case "rb":
			parser = &parsers.RubyParser{}
		case "js":
			parser = &parsers.JavaScriptParser{}
		default:
			// Skip unsupported file types for detailed parsing
			return nil
		}

		// Parse and store entities
		entities, err := parser.Parse(string(content))
		if err != nil {
			fmt.Printf("Warning: Could not parse %s: %v\n", path, err)
			stats.errors++
			return nil
		}

		stats.entitiesFound += len(entities)

		// Store file and entities in database
		if err := db.StoreFileAndEntities(path, ext, info.ModTime().Unix(), len(content), entities); err != nil {
			fmt.Printf("Warning: Could not store entities for %s: %v\n", path, err)
			stats.errors++
			// Continue anyway - don't return
		}

		return nil
	})

	fmt.Printf("\nScan complete!\n")
	fmt.Printf("Files scanned: %d\n", stats.filesScanned)
	fmt.Printf("Errors encountered: %d\n", stats.errors)

	return err
}
