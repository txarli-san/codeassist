package parsers

import (
	"regexp"
	"strings"
)

type JavaScriptParser struct{}

func (p *JavaScriptParser) Parse(filePath string, content string) ([]RichEntity, error) {
	var richEntities []RichEntity
	lines := strings.Split(content, "\n")

	funcRegex := regexp.MustCompile(`^\s*function\s+(\w+)\s*\(([^)]*)\)`)
	arrowFuncRegex := regexp.MustCompile(`^\s*(?:const|let|var)\s+(\w+)\s*=\s*(?:\(([^)]*)\)|([\w]+))\s*=>`)
	classRegex := regexp.MustCompile(`^\s*class\s+(\w+)(?:\s+extends\s+([\w.]+))?`)

	for i, line := range lines {
		var currentEntity Entity
		entityType := ""
		entityName := ""
		signature := ""
		description := ""
		lineStart := i + 1
		lineEnd := i + 1
		identified := false
		var entityContentLines []string

		if match := funcRegex.FindStringSubmatch(line); match != nil {
			identified = true
			entityType = "function"
			entityName = match[1]
			signature = match[2]
			lineEnd = p.findJSBlockEnd(lines, i, line)
			description = p.extractJSDocComments(lines, i)
		} else if match := arrowFuncRegex.FindStringSubmatch(line); match != nil {
			identified = true
			entityType = "arrow_function"
			entityName = match[1]
			if match[2] != "" {
				signature = match[2]
			} else if match[3] != "" {
				signature = match[3]
			}

			if strings.Contains(line, "{") {
				lineEnd = p.findJSBlockEnd(lines, i, line)
			} else {

				tempEndLine := i
				for j := i; j < len(lines); j++ {
					tempEndLine = j
					if strings.Contains(lines[j], ";") || (j > i && !strings.HasPrefix(strings.TrimSpace(lines[j]), "//") && lines[j] != "") {
						if !strings.Contains(lines[j], ";") && j > i {
							tempEndLine = j - 1
						}
						break
					}
					if j == len(lines)-1 {
						break
					}
				}
				lineEnd = tempEndLine
			}
			description = p.extractJSDocComments(lines, i)
		} else if match := classRegex.FindStringSubmatch(line); match != nil {
			identified = true
			entityType = "class"
			entityName = match[1]
			if len(match) > 2 && match[2] != "" {
				signature = match[2]
			}
			lineEnd = p.findJSBlockEnd(lines, i, line)
			description = p.extractJSDocComments(lines, i)
			if signature != "" {
				if description != "" {
					description += "\n"
				}
				description += "Extends: " + signature
			}

			methodRegex := regexp.MustCompile(`^\s*(static\s+)?(async\s+)?(get\s+|set\s+)?([\w$]+)\s*\(([^)]*)\)\s*\{`)
			constructorRegex := regexp.MustCompile(`^\s*constructor\s*\(([^)]*)\)\s*\{`)

			for j := i + 1; j < lineEnd; j++ {
				classLine := strings.TrimSpace(lines[j])
				var methodIsStatic, methodIsAsync bool
				var methodTypePrefix string

				methodMatch := methodRegex.FindStringSubmatch(classLine)
				if methodMatch == nil {
					methodMatch = constructorRegex.FindStringSubmatch(classLine)
					if methodMatch != nil {
						methodMatch = append(methodMatch[:1], "", "", "", "constructor", methodMatch[1])
					}
				}

				if methodMatch != nil {
					methodName := methodMatch[4]
					methodParams := methodMatch[5]

					if methodMatch[1] != "" {
						methodIsStatic = true
					}
					if methodMatch[2] != "" {
						methodIsAsync = true
					}
					if methodMatch[3] != "" {
						methodTypePrefix = strings.TrimSpace(methodMatch[3])
					}

					methodEndLine := p.findJSBlockEnd(lines, j, lines[j])
					methodDescription := p.extractJSDocComments(lines, j)
					methodFullName := entityName + "." + methodName
					if methodTypePrefix != "" {
						methodFullName = entityName + "." + methodTypePrefix + " " + methodName
					}
					if methodIsStatic {
						methodFullName = entityName + ".static " + methodName
					}
					if methodIsAsync {
						methodFullName = entityName + ".async " + methodName
					}

					richEntities = append(richEntities, RichEntity{
						Entity: Entity{
							Type:        "method",
							Name:        methodFullName,
							Signature:   methodParams,
							LineStart:   j + 1,
							LineEnd:     methodEndLine + 1,
							Content:     strings.Join(lines[j:methodEndLine+1], "\n"),
							Description: methodDescription,
						},
						FilePath: filePath,
					})
					j = methodEndLine
				}
			}
		}

		if identified {
			if lineEnd < lineStart-1 {
				lineEnd = lineStart - 1
			}
			if lineEnd >= len(lines) {
				lineEnd = len(lines) - 1
			}

			currentEntity.Type = entityType
			currentEntity.Name = entityName
			currentEntity.Signature = signature
			currentEntity.LineStart = lineStart
			currentEntity.LineEnd = lineEnd + 1

			if lineStart-1 <= lineEnd {
				entityContentLines = lines[lineStart-1 : lineEnd+1]
			} else {
				entityContentLines = []string{line}
			}
			currentEntity.Content = strings.Join(entityContentLines, "\n")
			currentEntity.Description = description

			richEntities = append(richEntities, RichEntity{
				Entity:   currentEntity,
				FilePath: filePath,
			})
		}
	}

	return richEntities, nil
}

func (p *JavaScriptParser) findJSBlockEnd(lines []string, startLine int, currentLineStr string) int {
	openBraces := 0
	inString := false
	var stringChar rune
	inCommentBlock := false
	inLineComment := false

	firstLineOpenBraces := 0
	firstLineHasOpenBrace := false
	for _, char := range currentLineStr {
		if char == '{' {
			firstLineOpenBraces++
			firstLineHasOpenBrace = true
		}
		if char == '}' {
			firstLineOpenBraces--
		}
	}

	if !firstLineHasOpenBrace && strings.Contains(currentLineStr, "=>") && !strings.Contains(currentLineStr, "{") {

		for i := startLine; i < len(lines); i++ {
			trimmedLine := strings.TrimSpace(lines[i])
			if strings.HasSuffix(trimmedLine, ";") || strings.HasSuffix(trimmedLine, ",") {
				return i
			}
			if i > startLine && trimmedLine != "" && !strings.HasPrefix(trimmedLine, "//") {
				return i - 1
			}
			if i == len(lines)-1 {
				return i
			}
		}
		return startLine
	}

	openBraces = firstLineOpenBraces

	for i := startLine; i < len(lines); i++ {
		lineToCheck := lines[i]
		if i == startLine {

			openBraceIndex := strings.Index(lineToCheck, "{")
			if openBraceIndex != -1 {
				lineToCheck = lineToCheck[openBraceIndex+1:]
			} else if openBraces <= 0 {
				return startLine
			}
		}

		for charIdx, char := range lineToCheck {
			if inLineComment {
				if char == '\n' {
					inLineComment = false
				}
				continue
			}
			if inCommentBlock {
				if char == '*' && charIdx+1 < len(lineToCheck) && lineToCheck[charIdx+1] == '/' {
					inCommentBlock = false
				}
				continue
			}
			if inString {
				if char == stringChar && (charIdx == 0 || lineToCheck[charIdx-1] != '\\') {
					inString = false
				}
				continue
			}

			if char == '/' && charIdx+1 < len(lineToCheck) {
				if lineToCheck[charIdx+1] == '/' {
					inLineComment = true

					break
				}
				if lineToCheck[charIdx+1] == '*' {
					inCommentBlock = true

					continue
				}
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
		inLineComment = false
	}
	return len(lines) - 1
}

func (p *JavaScriptParser) extractJSDocComments(lines []string, lineIndex int) string {
	var commentBlock []string
	inJSDoc := false

	for i := lineIndex - 1; i >= 0; i-- {
		trimmedLine := strings.TrimSpace(lines[i])
		if strings.HasSuffix(trimmedLine, "*/") {
			inJSDoc = true
			trimmedLine = strings.TrimSuffix(trimmedLine, "*/")
		}

		if inJSDoc {
			if strings.HasPrefix(trimmedLine, "/**") {
				trimmedLine = strings.TrimPrefix(trimmedLine, "/**")
				if trimmedLine != "" {
					commentBlock = append([]string{strings.TrimSpace(strings.TrimPrefix(trimmedLine, "* "))}, commentBlock...)
				}
				break
			}
			if strings.HasPrefix(trimmedLine, "*") {
				commentBlock = append([]string{strings.TrimSpace(strings.TrimPrefix(trimmedLine, "* "))}, commentBlock...)
			} else {
				break
			}
		} else {
			if trimmedLine == "" && len(commentBlock) > 0 {
				break
			}
			if strings.HasPrefix(trimmedLine, "//") {
				commentBlock = append([]string{strings.TrimSpace(strings.TrimPrefix(trimmedLine, "//"))}, commentBlock...)
			} else if trimmedLine != "" {
				break
			}
		}
		if lineIndex-i > 20 {
			break
		}
	}
	return strings.Join(commentBlock, "\n")
}
