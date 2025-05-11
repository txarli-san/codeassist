package parsers

type Entity struct {
	Type        string
	Name        string
	Signature   string
	LineStart   int
	LineEnd     int
	Content     string
	Description string
}

type CallRelation struct {
	TargetName           string
	TargetContext        string
	TargetResolvedID     *int64
	LineNumber           int
	Arguments            []string
	IsCrossFileOrPackage bool
}

type ImportInfo struct {
	Path  string `json:"path"`
	Alias string `json:"alias,omitempty"`
}

type RichEntity struct {
	Entity
	FilePath       string         `json:"-"`
	PackageName    string         `json:"packageName,omitempty"`
	SignatureJSON  string         `json:"signatureJson,omitempty"`
	ASTMetadata    string         `json:"astMetadata,omitempty"`
	Calls          []CallRelation `json:"calls,omitempty"`
	Imports        []ImportInfo   `json:"imports,omitempty"`
	LocalVariables []string       `json:"localVariables,omitempty"`
	ReceiverType   string         `json:"receiverType,omitempty"`
	ParamCount     int            `json:"paramCount,omitempty"`
	ReturnCount    int            `json:"returnCount,omitempty"`
}

type Parser interface {
	Parse(filePath string, content string) ([]RichEntity, error)
}
