package parsers

import (
	"regexp"
	"strings"
)

type HTMLParser struct{}

func (p *HTMLParser) Parse(filePath string, content string) ([]RichEntity, error) {
	var richEntities []RichEntity
	lines := strings.Split(content, "\n")

	titleRegex := regexp.MustCompile(`<title[^>]*>(.*?)</title>`)
	titleMatch := titleRegex.FindStringSubmatch(content)
	title := ""
	if len(titleMatch) > 1 {
		title = strings.TrimSpace(titleMatch[1])
	}

	isHTML := strings.Contains(strings.ToLower(content), "<!doctype html>") ||
		strings.Contains(strings.ToLower(content), "<html")

	var baseEntity Entity

	if !isHTML {
		baseEntity = Entity{
			Type:      "template",
			Name:      "unnamed_template",
			LineStart: 1,
			LineEnd:   len(lines),
			Content:   content,
		}
	} else {
		descriptionRegex := regexp.MustCompile(`<meta\s+name=["']description["']\s+content=["'](.*?)["']`)
		descriptionMatch := descriptionRegex.FindStringSubmatch(content)
		description := ""
		if len(descriptionMatch) > 1 {
			description = descriptionMatch[1]
		}

		keywordsRegex := regexp.MustCompile(`<meta\s+name=["']keywords["']\s+content=["'](.*?)["']`)
		keywordsMatch := keywordsRegex.FindStringSubmatch(content)
		if len(keywordsMatch) > 1 {
			if description != "" {
				description += "\n"
			}
			description += "Keywords: " + keywordsMatch[1]
		}

		entityName := title
		if entityName == "" {
			entityName = "html_document"
		}

		baseEntity = Entity{
			Type:        "html",
			Name:        entityName,
			LineStart:   1,
			LineEnd:     len(lines),
			Content:     content,
			Description: description,
		}
	}

	richEntities = append(richEntities, RichEntity{
		Entity:   baseEntity,
		FilePath: filePath,
	})

	return richEntities, nil
}
