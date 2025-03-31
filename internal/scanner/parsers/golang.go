package parsers

import (
	"regexp"
	"strings"
)

// GoParser implements Parser for Go language
type GoParser struct{}

// Parse extracts entities from Go code
func (p *GoParser) Parse(content string) ([]Entity, error) {
	var entities []Entity
	lines := strings.Split(content, "\n")

	// Extract package name
	packageRegex := regexp.MustCompile(`^package\s+(\w+)`)
	for i, line := range lines {
		if match := packageRegex.FindStringSubmatch(line); match != nil {
			entities = append(entities, Entity{
				Type:      "package",
				Name:      match[1],
				LineStart: i + 1,
				LineEnd:   i + 1,
				Content:   line,
			})
			break
		}
	}

	// Extract functions
	funcRegex := regexp.MustCompile(`^func\s+([\w\d]+)(?:\s*\(([^)]*)\))?\s*(?:\(([^)]*)\))?\s*(?:\{|$)`)
	methodRegex := regexp.MustCompile(`^func\s+\(([^)]*)\)\s*([\w\d]+)(?:\s*\(([^)]*)\))?\s*(?:\{|$)`)

	for i, line := range lines {
		if match := funcRegex.FindStringSubmatch(line); match != nil && !strings.HasPrefix(line, "\t") {
			// Found a function declaration
			name := match[1]
			params := ""
			if len(match) > 2 {
				params = match[2]
			}

			// Find function end
			openBraces := 0
			endLine := i
			for j := i; j < len(lines); j++ {
				openBraces += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
				if openBraces <= 0 && strings.Contains(lines[j], "}") {
					endLine = j
					break
				}
			}

			// Extract comments above function
			description := ""
			commentStart := i - 1
			for commentStart >= 0 && (strings.HasPrefix(strings.TrimSpace(lines[commentStart]), "//") || strings.TrimSpace(lines[commentStart]) == "") {
				if strings.HasPrefix(strings.TrimSpace(lines[commentStart]), "//") {
					description = strings.TrimPrefix(strings.TrimSpace(lines[commentStart]), "//") + "\n" + description
				}
				commentStart--
			}

			entities = append(entities, Entity{
				Type:        "function",
				Name:        name,
				Signature:   params,
				LineStart:   i + 1,
				LineEnd:     endLine + 1,
				Content:     strings.Join(lines[i:endLine+1], "\n"),
				Description: strings.TrimSpace(description),
			})
		} else if match := methodRegex.FindStringSubmatch(line); match != nil {
			// Found a method declaration
			receiver := match[1]
			name := match[2]
			params := ""
			if len(match) > 3 {
				params = match[3]
			}

			// Find method end
			openBraces := 0
			endLine := i
			for j := i; j < len(lines); j++ {
				openBraces += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
				if openBraces <= 0 && strings.Contains(lines[j], "}") {
					endLine = j
					break
				}
			}

			// Extract comments above method
			description := ""
			commentStart := i - 1
			for commentStart >= 0 && (strings.HasPrefix(strings.TrimSpace(lines[commentStart]), "//") || strings.TrimSpace(lines[commentStart]) == "") {
				if strings.HasPrefix(strings.TrimSpace(lines[commentStart]), "//") {
					description = strings.TrimPrefix(strings.TrimSpace(lines[commentStart]), "//") + "\n" + description
				}
				commentStart--
			}

			entities = append(entities, Entity{
				Type:        "method",
				Name:        name,
				Signature:   receiver + " " + params,
				LineStart:   i + 1,
				LineEnd:     endLine + 1,
				Content:     strings.Join(lines[i:endLine+1], "\n"),
				Description: strings.TrimSpace(description),
			})
		}
	}

	// Extract structs and interfaces
	typeRegex := regexp.MustCompile(`^type\s+([\w\d]+)\s+(struct|interface)\s*\{`)
	for i, line := range lines {
		if match := typeRegex.FindStringSubmatch(line); match != nil {
			name := match[1]
			typeName := match[2]

			// Find type end
			openBraces := 0
			endLine := i
			for j := i; j < len(lines); j++ {
				openBraces += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
				if openBraces <= 0 && strings.Contains(lines[j], "}") {
					endLine = j
					break
				}
			}

			// Extract comments above type
			description := ""
			commentStart := i - 1
			for commentStart >= 0 && (strings.HasPrefix(strings.TrimSpace(lines[commentStart]), "//") || strings.TrimSpace(lines[commentStart]) == "") {
				if strings.HasPrefix(strings.TrimSpace(lines[commentStart]), "//") {
					description = strings.TrimPrefix(strings.TrimSpace(lines[commentStart]), "//") + "\n" + description
				}
				commentStart--
			}

			entities = append(entities, Entity{
				Type:        typeName, // "struct" or "interface"
				Name:        name,
				LineStart:   i + 1,
				LineEnd:     endLine + 1,
				Content:     strings.Join(lines[i:endLine+1], "\n"),
				Description: strings.TrimSpace(description),
			})
		}
	}

	return entities, nil
}
