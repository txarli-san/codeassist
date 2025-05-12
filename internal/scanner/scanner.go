package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/txarli-san/codeassist/internal/scanner/parsers"
	"github.com/txarli-san/codeassist/internal/storage"
)

func ScanDirectory(rootDir, languages string, db *storage.DB) error {
	supportedLanguages := map[string]bool{
		"go":   true,
		"rb":   true,
		"js":   true,
		"html": true,
	}

	langMap := make(map[string]bool)
	if languages != "" && languages != "*" {
		langs := strings.Split(languages, ",")
		for _, l := range langs {
			lang := strings.TrimSpace(l)
			if supportedLanguages[lang] {
				langMap[lang] = true
			}
		}
	} else {
		langMap = supportedLanguages
	}

	fmt.Printf("Starting scan of directory: %s\n", rootDir)
	if languages == "" || languages == "*" {
		fmt.Println("Scanning for all supported languages")
	} else {
		fmt.Printf("Languages to scan: %s\n", languages)
	}

	var stats struct {
		filesScanned  int
		entitiesFound int
		errors        int
	}
	startTime := time.Now()

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("Error accessing path %s: %v\n", path, err)
			stats.errors++
			return filepath.SkipDir
		}

		if info.IsDir() {
			basename := filepath.Base(path)
			if strings.HasPrefix(basename, ".") && basename != "." && basename != ".." {
				return filepath.SkipDir
			}
			if basename == "node_modules" || basename == "vendor" || basename == ".bundle" || basename == "target" || basename == "build" || basename == "dist" || basename == "pkg" || basename == "tmp" || basename == "temp" {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Size() == 0 {
			return nil
		}

		if info.Size() > 2*1024*1024 {
			fmt.Printf("Skipping large file ( > 2MB ): %s\n", path)
			return nil
		}

		ext := strings.TrimPrefix(filepath.Ext(path), ".")

		langToUse := ""

		switch strings.ToLower(ext) {
		case "go":
			langToUse = "go"
		case "rb", "rake", "gemspec", "ru", "erb":
			langToUse = "rb"
		case "js", "jsx", "mjs", "cjs":
			langToUse = "js"
		case "ts", "tsx":
			langToUse = "js"
		case "html", "htm":
			langToUse = "html"
		}

		baseName := strings.ToLower(filepath.Base(path))
		if langToUse == "" {
			if baseName == "rakefile" || baseName == "gemfile" {
				langToUse = "rb"
			}
		}

		if langToUse == "" || !langMap[langToUse] {
			return nil
		}

		contentBytes, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("Warning: Could not read file %s: %v\n", path, err)
			stats.errors++
			return nil
		}
		contentStr := string(contentBytes)

		stats.filesScanned++
		if stats.filesScanned%100 == 0 {
			fmt.Printf("Scanned %d files...\n", stats.filesScanned)
		}

		if err := db.StoreWholeFile(path, langToUse, info.ModTime().Unix(), contentStr); err != nil {
			fmt.Printf("Warning: Could not store whole file %s: %v\n", path, err)
			stats.errors++

		}

		var parser parsers.Parser
		switch langToUse {
		case "go":
			parser = &parsers.GoParser{}
		case "rb":
			parser = &parsers.RubyParser{}
		case "js":
			parser = &parsers.JavaScriptParser{}
		case "html":
			parser = &parsers.HTMLParser{}
		default:
			return nil
		}

		parseStartTime := time.Now()
		richEntities, err := parser.Parse(path, contentStr)
		parseDuration := time.Since(parseStartTime)
		if parseDuration > 500*time.Millisecond {
			fmt.Printf("Slow parse for %s: %v\n", path, parseDuration)
		}

		if err != nil {
			fmt.Printf("Warning: Could not parse %s: %v\n", path, err)
			stats.errors++
			return nil
		}

		stats.entitiesFound += len(richEntities)

		if err := db.StoreFileAndEntities(path, langToUse, info.ModTime().Unix(), len(contentBytes), richEntities); err != nil {
			fmt.Printf("Warning: Could not store entities for %s: %v\n", path, err)
			stats.errors++
		}

		return nil
	})

	duration := time.Since(startTime)
	fmt.Printf("\nScan complete!\n")
	fmt.Printf("Total files scanned: %d\n", stats.filesScanned)
	fmt.Printf("Total entities found: %d\n", stats.entitiesFound)
	fmt.Printf("Errors encountered: %d\n", stats.errors)
	fmt.Printf("Scan duration: %s\n", duration)

	return err
}
