package parsers

import (
	"regexp"
	"strings"
)

type RubyParser struct{}

func (p *RubyParser) Parse(filePath string, content string) ([]RichEntity, error) {
	var richEntities []RichEntity
	lines := strings.Split(content, "\n")

	classRegex := regexp.MustCompile(`^\s*class\s+([\w:]+)(?:\s*<\s*([\w:]+))?`)
	for i, line := range lines {
		if match := classRegex.FindStringSubmatch(line); match != nil {
			name := match[1]
			parentClass := ""
			if len(match) > 2 && match[2] != "" {
				parentClass = match[2]
			}

			endLine := p.findRubyBlockEnd(lines, i)
			description := p.extractRubyComments(lines, i)

			contentStartLine := i
			contentEndLine := endLine
			if contentEndLine < contentStartLine {
				contentEndLine = contentStartLine
			}
			if contentEndLine >= len(lines) {
				contentEndLine = len(lines) - 1
			}

			fullContent := ""
			if contentStartLine <= contentEndLine && contentStartLine < len(lines) && contentEndLine < len(lines) {
				fullContent = strings.Join(lines[contentStartLine:contentEndLine+1], "\n")
			} else if contentStartLine == contentEndLine && contentStartLine < len(lines) {
				fullContent = lines[contentStartLine]
			}

			if parentClass != "" {
				if description != "" {
					description += "\n"
				}
				description += "Inherits from: " + parentClass
			}

			richEntities = append(richEntities, RichEntity{
				Entity: Entity{
					Type:        "class",
					Name:        name,
					Signature:   parentClass,
					LineStart:   i + 1,
					LineEnd:     endLine + 1,
					Content:     fullContent,
					Description: strings.TrimSpace(description),
				},
				FilePath: filePath,
			})
		}
	}

	moduleRegex := regexp.MustCompile(`^\s*module\s+([\w:]+)`)
	for i, line := range lines {
		if match := moduleRegex.FindStringSubmatch(line); match != nil {
			name := match[1]
			endLine := p.findRubyBlockEnd(lines, i)
			description := p.extractRubyComments(lines, i)

			contentStartLine := i
			contentEndLine := endLine
			if contentEndLine < contentStartLine {
				contentEndLine = contentStartLine
			}
			if contentEndLine >= len(lines) {
				contentEndLine = len(lines) - 1
			}

			fullContent := ""
			if contentStartLine <= contentEndLine && contentStartLine < len(lines) && contentEndLine < len(lines) {
				fullContent = strings.Join(lines[contentStartLine:contentEndLine+1], "\n")
			} else if contentStartLine == contentEndLine && contentStartLine < len(lines) {
				fullContent = lines[contentStartLine]
			}

			richEntities = append(richEntities, RichEntity{
				Entity: Entity{
					Type:        "module",
					Name:        name,
					LineStart:   i + 1,
					LineEnd:     endLine + 1,
					Content:     fullContent,
					Description: strings.TrimSpace(description),
				},
				FilePath: filePath,
			})
		}
	}

	methodRegex := regexp.MustCompile(`^\s*def\s+((?:self\.)?[\w!?]+(?:[=!?])?|\[\]=?|[!~@]|(?:[+\-*/%&|^<>=]=?)|(?:[<>=!]=~)|[<>=!]=|[+\-*/%&|^<>]{1,2}|` + "`" + `)(?:\s*\(?([^)]*)\)?)?`)
	for i, line := range lines {
		if match := methodRegex.FindStringSubmatch(line); match != nil {
			name := match[1]
			params := ""
			if len(match) >= 3 {
				params = strings.TrimSpace(match[2])
			}

			endLine := p.findRubyBlockEnd(lines, i)
			description := p.extractRubyComments(lines, i)

			contentStartLine := i
			contentEndLine := endLine
			if contentEndLine < contentStartLine {
				contentEndLine = contentStartLine
			}
			if contentEndLine >= len(lines) {
				contentEndLine = len(lines) - 1
			}

			fullContent := ""
			if contentStartLine <= contentEndLine && contentStartLine < len(lines) && contentEndLine < len(lines) {
				fullContent = strings.Join(lines[contentStartLine:contentEndLine+1], "\n")
			} else if contentStartLine == contentEndLine && contentStartLine < len(lines) {
				fullContent = lines[contentStartLine]
			}

			richEntities = append(richEntities, RichEntity{
				Entity: Entity{
					Type:        "method",
					Name:        name,
					Signature:   params,
					LineStart:   i + 1,
					LineEnd:     endLine + 1,
					Content:     fullContent,
					Description: strings.TrimSpace(description),
				},
				FilePath: filePath,
			})
		}
	}

	return richEntities, nil
}

func (p *RubyParser) isCharEscaped(line string, index int) bool {
	if index == 0 {
		return false
	}
	slashes := 0
	for i := index - 1; i >= 0; i-- {
		if line[i] == '\\' {
			slashes++
		} else {
			break
		}
	}
	return slashes%2 == 1
}

func (p *RubyParser) findRubyBlockEnd(lines []string, startLine int) int {
	balance := 0
	inSingleString := false
	inDoubleString := false
	inRegex := false
	inHeredoc := false
	var heredocTerminator string
	heredocCanBeIndented := false

	blockOpeners := []string{"class", "module", "def", "if", "unless", "while", "until", "for", "case", "begin"}
	endKeyword := "end"

	for i := startLine; i < len(lines); i++ {
		line := lines[i]

		if inHeredoc {
			trimmedLine := strings.TrimSpace(line)
			if heredocCanBeIndented {
				if trimmedLine == heredocTerminator {
					inHeredoc = false
				}
			} else {
				if line == heredocTerminator {
					inHeredoc = false
				}
			}
			if i == startLine && balance == 0 {
				balance = 1
			}
			if inHeredoc {
				continue
			}
		}

		var effectiveLineForBalance strings.Builder
		var prevChar rune = 0
		lineRune := []rune(line)

		for j := 0; j < len(lineRune); j++ {
			char := lineRune[j]

			if char == '#' && !inSingleString && !inDoubleString && !inRegex {
				break
			}

			if inSingleString {
				effectiveLineForBalance.WriteRune('S')
				if char == '\'' && prevChar != '\\' {
					inSingleString = false
				}
			} else if inDoubleString {
				effectiveLineForBalance.WriteRune('S')
				if char == '"' && prevChar != '\\' {
					if j+1 < len(lineRune) && lineRune[j+1] == '"' && prevChar == '"' {
						prevChar = 0
						j++
						continue
					}
					inDoubleString = false
				}
			} else if inRegex {
				effectiveLineForBalance.WriteRune('R')
				if char == '/' && prevChar != '\\' {
					inRegex = false
					for k := j + 1; k < len(lineRune); k++ {
						if strings.ContainsRune("gimxouesn", lineRune[k]) {
							effectiveLineForBalance.WriteRune('R')
						} else {
							break
						}
					}
				}
			} else {
				effectiveLineForBalance.WriteRune(char)
				if char == '\'' {
					inSingleString = true
				} else if char == '"' {
					inDoubleString = true
				} else if char == '/' {
					isDiv := false
					if j > 0 {
						prevActualCharIndex := -1
						for k := j - 1; k >= 0; k-- {
							if lineRune[k] != ' ' && lineRune[k] != '\t' {
								prevActualCharIndex = k
								break
							}
						}
						if prevActualCharIndex != -1 {
							prevActualChar := lineRune[prevActualCharIndex]
							if (prevActualChar >= '0' && prevActualChar <= '9') ||
								(prevActualChar >= 'a' && prevActualChar <= 'z') ||
								(prevActualChar >= 'A' && prevActualChar <= 'Z') ||
								prevActualChar == ')' || prevActualChar == ']' || prevActualChar == '_' {
								isDiv = true
							}
						}
					}
					if !isDiv {
						inRegex = true
					}
				} else if char == '<' && j+1 < len(lineRune) && lineRune[j+1] == '<' {
					match := regexp.MustCompile(`<<(-)?(['"]?)(\w+)\2`).FindStringSubmatch(line[j:])
					if len(match) > 0 {
						inHeredoc = true
						heredocTerminator = match[3]
						heredocCanBeIndented = (match[1] == "-")
						effectiveLineForBalance.WriteRune('<')
						j++
						break
					}
				}
			}
			prevChar = char
		}

		processedLine := strings.TrimSpace(effectiveLineForBalance.String())

		if i == startLine {
			balance = 1
		} else {
			words := regexp.MustCompile(`[^\s{};,()\[\]]+`).FindAllString(processedLine, -1)
			for _, word := range words {
				for _, kw := range blockOpeners {
					if word == kw {
						balance++
					}
				}
				if word == "do" {

					isBlockArgDo := false
					if pipeIndex := strings.Index(processedLine, "|"); pipeIndex != -1 && strings.Index(processedLine, "do") < pipeIndex {
						isBlockArgDo = true
					}
					if !isBlockArgDo {

						trimmedOriginalLine := strings.TrimSpace(lines[i])
						originalDoIndex := strings.Index(trimmedOriginalLine, "do")
						if originalDoIndex != -1 {
							if originalDoIndex == 0 || !isRubyIdentChar(rune(trimmedOriginalLine[originalDoIndex-1])) {
								if len(trimmedOriginalLine) == originalDoIndex+2 || !isRubyIdentChar(rune(trimmedOriginalLine[originalDoIndex+2])) {
									balance++
								}
							}
						}

					} else {
						balance++
					}

				}
				if word == endKeyword {
					balance--
				}
			}
			for _, char := range processedLine {
				if char == '{' {
					balance++
				} else if char == '}' {
					balance--
				}
			}
		}

		if balance <= 0 && i >= startLine {
			trimmedOriginalLine := strings.TrimSpace(lines[i])
			commentIdx := strings.Index(trimmedOriginalLine, "#")
			lineToCheckForEnd := trimmedOriginalLine
			if commentIdx != -1 {
				lineToCheckForEnd = strings.TrimSpace(trimmedOriginalLine[:commentIdx])
			}

			if lineToCheckForEnd == "end" || strings.HasPrefix(lineToCheckForEnd, "end ") || strings.HasSuffix(lineToCheckForEnd, " end") || strings.Contains(lineToCheckForEnd, " end ") ||
				lineToCheckForEnd == "}" || strings.HasPrefix(lineToCheckForEnd, "} ") || strings.HasSuffix(lineToCheckForEnd, " }") || strings.Contains(lineToCheckForEnd, " } ") {

				isLikelyEnd := true
				if i+1 < len(lines) {
					nextLineTrimmed := strings.TrimSpace(lines[i+1])
					if strings.HasPrefix(nextLineTrimmed, "else") || strings.HasPrefix(nextLineTrimmed, "elsif") ||
						strings.HasPrefix(nextLineTrimmed, "when") || strings.HasPrefix(nextLineTrimmed, "rescue") ||
						strings.HasPrefix(nextLineTrimmed, "ensure") {

						if balance == 0 {
						} else {
							isLikelyEnd = false
						}
					}
				}
				if isLikelyEnd {
					return i
				}
			}
			if balance == 0 && i >= startLine {
				return i
			}

		}
	}
	return len(lines) - 1
}

func isRubyIdentChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func (p *RubyParser) extractRubyComments(lines []string, lineIndex int) string {
	var commentLines []string
	for i := lineIndex - 1; i >= 0; i-- {
		trimmedLine := strings.TrimSpace(lines[i])
		if trimmedLine == "" {
			if len(commentLines) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmedLine, "#") {
			commentLines = append([]string{strings.TrimSpace(strings.TrimPrefix(trimmedLine, "#"))}, commentLines...)
		} else {
			break
		}
	}
	return strings.Join(commentLines, "\n")
}
