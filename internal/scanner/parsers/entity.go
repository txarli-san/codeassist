package parsers

// Entity represents a code structure like function, class, etc.
type Entity struct {
	Type        string // function, method, class, struct, etc.
	Name        string // name of the entity
	Signature   string // parameters or type information
	LineStart   int    // starting line in file
	LineEnd     int    // ending line in file
	Content     string // full content of the entity
	Description string // documentation or comments
}

// Parser interface for language-specific parsers
type Parser interface {
	Parse(content string) ([]Entity, error)
}
