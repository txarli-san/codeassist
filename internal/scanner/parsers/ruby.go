package parsers

import (
	"regexp"
	"strings"
)

// RubyParser implements Parser for Ruby language
type RubyParser struct{}

// Parse extracts entities from Ruby code
func (p *RubyParser) Parse(content string) ([]Entity, error) {
	var entities []Entity
	lines := strings.Split(content, "\n")

	// Extract classes
	classRegex := regexp.MustCompile(`^\s*class\s+(\w+)(?:\s+<\s+(\w+))?`)
	for i, line := range lines {
		if match := classRegex.FindStringSubmatch(line); match != nil {
			name := match[1]
			parentClass := ""
			if len(match) > 2 {
				parentClass = match[2]
			}

			// Find class end
			endLine := findRubyBlockEnd(lines, i, "class")

			// Extract comments above class
			description := extractRubyComments(lines, i)
			if parentClass != "" {
				description += "\nInherits from: " + parentClass
			}

			entities = append(entities, Entity{
				Type:        "class",
				Name:        name,
				Signature:   parentClass,
				LineStart:   i + 1,
				LineEnd:     endLine + 1,
				Content:     strings.Join(lines[i:endLine+1], "\n"),
				Description: strings.TrimSpace(description),
			})
		}
	}

	// Extract modules
	moduleRegex := regexp.MustCompile(`^\s*module\s+(\w+)`)
	for i, line := range lines {
		if match := moduleRegex.FindStringSubmatch(line); match != nil {
			name := match[1]

			// Find module end
			endLine := findRubyBlockEnd(lines, i, "module")

			// Extract comments above module
			description := extractRubyComments(lines, i)

			entities = append(entities, Entity{
				Type:        "module",
				Name:        name,
				LineStart:   i + 1,
				LineEnd:     endLine + 1,
				Content:     strings.Join(lines[i:endLine+1], "\n"),
				Description: strings.TrimSpace(description),
			})
		}
	}

	// Extract methods
	methodRegex := regexp.MustCompile(`^\s*def\s+(\w+|\w+[?!]|[+-/%*<>=]+)(?:\(([^)]*)\))?`)
	for i, line := range lines {
		if match := methodRegex.FindStringSubmatch(line); match != nil {
			name := match[1]
			params := ""
			if len(match) > 2 {
				params = match[2]
			}

			// Find method end
			endLine := findRubyBlockEnd(lines, i, "def")

			// Extract comments above method
			description := extractRubyComments(lines, i)

			entities = append(entities, Entity{
				Type:        "method",
				Name:        name,
				Signature:   params,
				LineStart:   i + 1,
				LineEnd:     endLine + 1,
				Content:     strings.Join(lines[i:endLine+1], "\n"),
				Description: strings.TrimSpace(description),
			})
		}
	}

	return entities, nil
}

// Helper function to find the end of a Ruby block
func findRubyBlockEnd(lines []string, startLine int, blockType string) int {
	endKeyword := "end"
	stack := 1

	for i := startLine + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Count block starts
		if strings.HasPrefix(line, "class ") ||
			strings.HasPrefix(line, "module ") ||
			strings.HasPrefix(line, "def ") ||
			strings.HasPrefix(line, "if ") ||
			strings.HasPrefix(line, "unless ") ||
			strings.HasPrefix(line, "case ") ||
			strings.HasPrefix(line, "while ") ||
			strings.HasPrefix(line, "until ") ||
			strings.HasPrefix(line, "for ") ||
			strings.HasPrefix(line, "begin") ||
			strings.HasPrefix(line, "do") {
			stack++
		}

		// Count block ends
		if line == endKeyword || strings.HasSuffix(line, " "+endKeyword) || strings.HasSuffix(line, ";"+endKeyword) {
			stack--
			if stack == 0 {
				return i
			}
		}
	}

	// Default to the last line if no end found
	return len(lines) - 1
}

// Helper function to extract comments above a line
func extractRubyComments(lines []string, lineIndex int) string {
	description := ""
	commentStart := lineIndex - 1

	for commentStart >= 0 && (strings.HasPrefix(strings.TrimSpace(lines[commentStart]), "#") || strings.TrimSpace(lines[commentStart]) == "") {
		if strings.HasPrefix(strings.TrimSpace(lines[commentStart]), "#") {
			description = strings.TrimPrefix(strings.TrimSpace(lines[commentStart]), "#") + "\n" + description
		}
		commentStart--
	}

	return strings.TrimSpace(description)
}
