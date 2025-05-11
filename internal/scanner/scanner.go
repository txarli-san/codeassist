package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("Error accessing path %s: %v\n", path, err)
			stats.errors++
			return filepath.SkipDir
		}

		if info.IsDir() {
			basename := filepath.Base(path)
			if basename == ".git" || basename == "node_modules" || basename == "vendor" || basename == ".bundle" {
				fmt.Printf("Skipping directory: %s\n", path)
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.TrimPrefix(filepath.Ext(path), ".")
		originalExt := ext

		if strings.HasSuffix(path, ".html.erb") {
			ext = "html"
		} else if strings.HasSuffix(path, ".jsx") || strings.HasSuffix(path, ".tsx") {
			ext = "js"
		} else if strings.HasSuffix(path, ".rake") || strings.HasSuffix(path, ".gemspec") {
			ext = "rb"
		}

		if !langMap[ext] {
			if originalExt == "htm" && langMap["html"] {
				ext = "html"
			} else if (originalExt == "jsx" || originalExt == "tsx") && langMap["js"] {
				ext = "js"
			} else if (originalExt == "rake" || originalExt == "gemspec") && langMap["rb"] {
				ext = "rb"
			} else {
				return nil
			}
		}

		contentBytes, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("Warning: Could not read file %s: %v\n", path, err)
			stats.errors++
			return nil
		}
		contentStr := string(contentBytes)

		fmt.Printf("Scanning file: %s (lang: %s)\n", path, ext)
		stats.filesScanned++

		if err := db.StoreWholeFile(path, ext, info.ModTime().Unix(), contentStr); err != nil {
			fmt.Printf("Warning: Could not store whole file %s: %v\n", path, err)
			stats.errors++
		}

		var parser parsers.Parser
		switch ext {
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

		richEntities, err := parser.Parse(path, contentStr)
		if err != nil {
			fmt.Printf("Warning: Could not parse %s: %v\n", path, err)
			stats.errors++
			return nil
		}

		stats.entitiesFound += len(richEntities)

		var plainEntities []parsers.Entity
		for _, re := range richEntities {
			plainEntities = append(plainEntities, re.Entity)
		}

		if err := db.StoreFileAndEntities(path, ext, info.ModTime().Unix(), len(contentBytes), plainEntities); err != nil {
			fmt.Printf("Warning: Could not store entities for %s: %v\n", path, err)
			stats.errors++
		}

		return nil
	})

	fmt.Printf("\nScan complete!\n")
	fmt.Printf("Files scanned: %d\n", stats.filesScanned)
	fmt.Printf("Entities found: %d\n", stats.entitiesFound)
	fmt.Printf("Errors encountered: %d\n", stats.errors)

	return err
}
