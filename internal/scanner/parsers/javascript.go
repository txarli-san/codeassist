package parsers

import (
	"regexp"
	"strings"
)

// JavaScriptParser implements Parser for JavaScript language
type JavaScriptParser struct{}

// Parse extracts entities from JavaScript code
func (p *JavaScriptParser) Parse(content string) ([]Entity, error) {
	var entities []Entity
	lines := strings.Split(content, "\n")

	// Extract functions
	funcRegex := regexp.MustCompile(`^\s*function\s+(\w+)\s*\(([^)]*)\)`)
	arrowFuncRegex := regexp.MustCompile(`^\s*const\s+(\w+)\s*=\s*(?:\(([^)]*)\)|(\w+))\s*=>`)
	methodRegex := regexp.MustCompile(`^\s*(\w+)\s*(?:\([^)]*\))?\s*\{`)
	classRegex := regexp.MustCompile(`^\s*class\s+(\w+)(?:\s+extends\s+(\w+))?`)

	for i, line := range lines {
		// Check for regular functions
		if match := funcRegex.FindStringSubmatch(line); match != nil {
			name := match[1]
			params := match[2]

			// Find function end
			endLine := findJSBlockEnd(lines, i)

			// Extract JSDoc comments
			description := extractJSDocComments(lines, i)

			entities = append(entities, Entity{
				Type:        "function",
				Name:        name,
				Signature:   params,
				LineStart:   i + 1,
				LineEnd:     endLine + 1,
				Content:     strings.Join(lines[i:endLine+1], "\n"),
				Description: description,
			})
		}

		// Check for arrow functions
		if match := arrowFuncRegex.FindStringSubmatch(line); match != nil {
			name := match[1]
			params := ""
			if match[2] != "" {
				params = match[2]
			} else if match[3] != "" {
				params = match[3]
			}

			// Handle single-line arrow functions
			endLine := i
			if !strings.Contains(line, ";") && !strings.Contains(line, "=>{}") {
				for j := i; j < len(lines) && j < i+10; j++ {
					if strings.Contains(lines[j], ";") {
						endLine = j
						break
					}
				}
			} else if strings.Contains(line, "=>") && strings.Contains(line, "{") {
				endLine = findJSBlockEnd(lines, i)
			}

			// Extract JSDoc comments
			description := extractJSDocComments(lines, i)

			entities = append(entities, Entity{
				Type:        "arrow_function",
				Name:        name,
				Signature:   params,
				LineStart:   i + 1,
				LineEnd:     endLine + 1,
				Content:     strings.Join(lines[i:endLine+1], "\n"),
				Description: description,
			})
		}

		// Check for classes
		if match := classRegex.FindStringSubmatch(line); match != nil {
			name := match[1]
			extends := ""
			if len(match) > 2 {
				extends = match[2]
			}

			// Find class end
			endLine := findJSBlockEnd(lines, i)

			// Extract JSDoc comments
			description := extractJSDocComments(lines, i)
			if extends != "" {
				description += "\nExtends: " + extends
			}

			entities = append(entities, Entity{
				Type:        "class",
				Name:        name,
				Signature:   extends,
				LineStart:   i + 1,
				LineEnd:     endLine + 1,
				Content:     strings.Join(lines[i:endLine+1], "\n"),
				Description: description,
			})

			// Extract methods within the class
			for j := i + 1; j < endLine; j++ {
				if match := methodRegex.FindStringSubmatch(lines[j]); match != nil && !strings.HasPrefix(strings.TrimSpace(lines[j]), "if") && !strings.HasPrefix(strings.TrimSpace(lines[j]), "while") && !strings.HasPrefix(strings.TrimSpace(lines[j]), "for") {
					methodName := match[1]

					// Skip if this is a JS keyword
					if methodName == "if" || methodName == "else" || methodName == "while" || methodName == "for" || methodName == "switch" || methodName == "case" || methodName == "default" {
						continue
					}

					// Find method end
					methodEndLine := findJSBlockEnd(lines, j)

					// Extract JSDoc comments
					methodDescription := extractJSDocComments(lines, j)

					entities = append(entities, Entity{
						Type:        "method",
						Name:        name + "." + methodName,
						LineStart:   j + 1,
						LineEnd:     methodEndLine + 1,
						Content:     strings.Join(lines[j:methodEndLine+1], "\n"),
						Description: methodDescription,
					})

					// Skip to after the method
					j = methodEndLine
				}
			}
		}
	}

	return entities, nil
}

// Helper function to find the end of a JS block
func findJSBlockEnd(lines []string, startLine int) int {
	openBraces := 0
	inString := false
	stringChar := ' '

	// Count opening braces in the start line
	for _, char := range lines[startLine] {
		if inString {
			if char == rune(stringChar) {
				inString = false
			}
			continue
		}

		if char == '"' || char == '\'' || char == '`' {
			inString = true
			stringChar = char
			continue
		}

		if char == '{' {
			openBraces++
		} else if char == '}' {
			openBraces--
		}
	}

	for i := startLine + 1; i < len(lines); i++ {
		for _, char := range lines[i] {
			if inString {
				if char == rune(stringChar) {
					inString = false
				}
				continue
			}

			if char == '"' || char == '\'' || char == '`' {
				inString = true
				stringChar = char
				continue
			}

			if char == '{' {
				openBraces++
			} else if char == '}' {
				openBraces--
				if openBraces <= 0 {
					return i
				}
			}
		}
	}

	return len(lines) - 1
}

// Helper function to extract JSDoc comments
func extractJSDocComments(lines []string, lineIndex int) string {
	description := ""
	inJSDoc := false
	jsDocStart := -1

	// Look for JSDoc style comments (/**...*/)
	for i := lineIndex - 1; i >= 0 && i >= lineIndex-20; i-- {
		trimmedLine := strings.TrimSpace(lines[i])

		if !inJSDoc && strings.HasSuffix(trimmedLine, "*/") {
			inJSDoc = true
			jsDocStart = i
			continue
		}

		if inJSDoc && strings.HasPrefix(trimmedLine, "/**") {
			// Extract all lines in the JSDoc block
			for j := i; j <= jsDocStart; j++ {
				// Strip comment syntax and leading asterisks
				line := strings.TrimSpace(lines[j])
				line = strings.TrimPrefix(line, "/**")
				line = strings.TrimPrefix(line, "*/")
				line = strings.TrimPrefix(line, "*")
				line = strings.TrimSpace(line)

				if line != "" {
					description += line + "\n"
				}
			}
			break
		}
	}

	// If no JSDoc found, look for single-line comments
	if description == "" {
		for i := lineIndex - 1; i >= 0 && i >= lineIndex-5; i-- {
			trimmedLine := strings.TrimSpace(lines[i])

			if strings.HasPrefix(trimmedLine, "//") {
				comment := strings.TrimPrefix(trimmedLine, "//")
				description = strings.TrimSpace(comment) + "\n" + description
			} else if trimmedLine == "" {
				continue
			} else {
				break
			}
		}
	}

	return strings.TrimSpace(description)
}
