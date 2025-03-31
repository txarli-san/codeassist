package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/txarli-san/codeassist/internal/scanner/parsers"
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
