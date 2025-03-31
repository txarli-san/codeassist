package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/txarli-san/codeassist/internal/scanner/parsers"
	"github.com/txarli-san/codeassist/internal/storage"
)

// OllamaClient handles interaction with Ollama API
type OllamaClient struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaClient creates a new Ollama client
func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// GenerateResponse generates a response using Ollama
func (c *OllamaClient) GenerateResponse(query string, entities []parsers.Entity) (string, error) {
	// Build prompt with context
	prompt := buildPromptWithContext(query, entities)

	// Create request body - add stream: false to explicitly disable streaming
	reqBody, err := json.Marshal(map[string]interface{}{
		"model":  c.model,
		"prompt": prompt,
		"stream": false, // Explicitly disable streaming
	})
	if err != nil {
		return "", err
	}

	fmt.Printf("Using Ollama model: %s\n", c.model)
	fmt.Println("Sending request to Ollama...")

	// Increase timeout for large responses
	client := &http.Client{
		Timeout: 5 * time.Minute, // Much longer timeout for complex queries
	}

	// Make API request
	resp, err := client.Post(fmt.Sprintf("%s/api/generate", c.baseURL),
		"application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Sprintf("Error connecting to Ollama: %v\nMake sure Ollama is running on %s", err, c.baseURL), err
	}
	defer resp.Body.Close()

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Sprintf("Error parsing Ollama response: %v", err), err
	}

	// Extract response text
	response, ok := result["response"].(string)
	if !ok {
		return fmt.Sprintf("Unexpected response format from Ollama"),
			fmt.Errorf("unexpected response format")
	}

	return response, nil
}

func (c *OllamaClient) GenerateStreamingResponse(query string, db *storage.DB) error {
	// Find relevant code sections instead of files
	sections, err := db.FindRelevantCodeSections(query, 5)
	if err != nil {
		return err
	}

	// Build prompt with context from sections
	var sb strings.Builder

	// Add system instruction
	sb.WriteString("You are a helpful coding assistant with access to parts of the codebase. ")
	sb.WriteString("Answer questions using the provided code context when relevant.\n\n")

	// Add code context
	if len(sections) > 0 {
		sb.WriteString("Here are relevant parts of the codebase:\n\n")

		for i, section := range sections {
			sb.WriteString(fmt.Sprintf("--- File: %s (Lines %d-%d) ---\n",
				section.Path, section.StartLine, section.EndLine))

			sb.WriteString(fmt.Sprintf("```%s\n%s\n```\n\n", section.Language, section.Content))

			// Limit context size
			if i >= 4 {
				sb.WriteString("...(additional code sections omitted for brevity)...\n\n")
				break
			}
		}

		// Add relationship information if available
		sb.WriteString("Relevant code relationships:\n")
		for i, section := range sections {
			if i >= 3 { // Limit to top 3 for brevity
				break
			}

			// Extract file basename for clarity
			basename := filepath.Base(section.Path)

			// Add relationship info
			if relatedEntities, err := db.FindRelatedEntities(section.Path); err == nil && len(relatedEntities) > 0 {
				sb.WriteString(fmt.Sprintf("- %s: ", basename))
				for j, entity := range relatedEntities {
					if j > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(fmt.Sprintf("%s (%s)", entity.Name, entity.Type))
					if j >= 2 {
						sb.WriteString("...")
						break
					}
				}
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("No relevant code sections found. Try scanning your codebase first with:\n")
		sb.WriteString("./bin/codeassist scan -path=/path/to/your/project -langs=go,rb,js\n\n")
	}

	// Add user query
	sb.WriteString("User question: " + query + "\n\n")
	sb.WriteString("Please provide a detailed and comprehensive response based on the code context above.")

	prompt := sb.String()

	// Create request body
	reqBody, err := json.Marshal(map[string]interface{}{
		"model":  c.model,
		"prompt": prompt,
		"stream": true,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Using Ollama model: %s\n", c.model)
	fmt.Println("Found", len(sections), "relevant code sections")
	fmt.Println("Sending streaming request to Ollama...")
	fmt.Println("\nResponse:")

	// Make API request
	client := &http.Client{
		Timeout: 10 * time.Minute,
	}

	resp, err := client.Post(fmt.Sprintf("%s/api/generate", c.baseURL),
		"application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		fmt.Printf("Error connecting to Ollama: %v\n", err)
		return err
	}
	defer resp.Body.Close()

	// Read the response line by line
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse the JSON response
		var streamResp map[string]interface{}
		if err := json.Unmarshal([]byte(line), &streamResp); err != nil {
			continue // Skip invalid JSON
		}

		// Extract and print token
		if token, ok := streamResp["response"].(string); ok {
			fmt.Print(token)
			// Flush stdout to ensure immediate display
			os.Stdout.Sync()
		}

		// Check if we're done
		if done, ok := streamResp["done"].(bool); ok && done {
			fmt.Println() // Add newline at the end
			break
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("\nError reading response: %v\n", err)
		return err
	}

	return nil
}

// ListModels lists available models from Ollama
func (c *OllamaClient) ListModels() ([]string, error) {
	resp, err := c.client.Get(fmt.Sprintf("%s/api/tags", c.baseURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := []string{}
	modelsData, ok := result["models"].([]interface{})
	if !ok {
		return models, nil
	}

	for _, model := range modelsData {
		modelData, ok := model.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := modelData["name"].(string)
		if !ok {
			continue
		}

		models = append(models, name)
	}

	return models, nil
}

// buildPromptWithContext builds a prompt with code context
func buildPromptWithContext(query string, entities []parsers.Entity) string {
	var sb strings.Builder

	// Add system instruction
	sb.WriteString("You are a helpful coding assistant with access to parts of the codebase. ")
	sb.WriteString("Answer questions using the provided code context when relevant. ")
	sb.WriteString("When you refer to code, make sure to specify the file or function name.")
	sb.WriteString("\n\n")

	// Add code context
	if len(entities) > 0 {
		sb.WriteString("Here are relevant parts of the codebase:\n\n")

		for i, entity := range entities {
			sb.WriteString(fmt.Sprintf("--- %s: %s ---\n", entity.Type, entity.Name))
			if entity.Description != "" {
				sb.WriteString(fmt.Sprintf("Description: %s\n", entity.Description))
			}
			sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", entity.Content))

			// Limit context size
			if i >= 4 {
				sb.WriteString("...(additional context omitted for brevity)...\n\n")
				break
			}
		}
	} else {
		sb.WriteString("No specific code context was found for your query.\n\n")
	}

	// Add user query
	sb.WriteString("User question: " + query + "\n\n")
	sb.WriteString("Please provide a helpful response based on the code context above.")

	return sb.String()
}
