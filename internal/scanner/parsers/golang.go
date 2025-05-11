package parsers

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

type GoParser struct{}

func (p *GoParser) Parse(filePath string, content string) ([]RichEntity, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var richEntities []RichEntity
	var currentPackageName string
	var currentFileImports []ImportInfo

	if node.Name != nil {
		currentPackageName = node.Name.Name
	}

	for _, importSpec := range node.Imports {
		var alias string
		if importSpec.Name != nil {
			alias = importSpec.Name.Name
		}
		path := ""
		if importSpec.Path != nil {
			path = strings.Trim(importSpec.Path.Value, `"`)
		}
		currentFileImports = append(currentFileImports, ImportInfo{
			Path:  path,
			Alias: alias,
		})
	}

	ast.Inspect(node, func(n ast.Node) bool {
		re := RichEntity{
			FilePath:    filePath,
			PackageName: currentPackageName,
			Imports:     currentFileImports,
		}
		var entityIdentifiable bool

		switch x := n.(type) {
		case *ast.FuncDecl:
			entityIdentifiable = true
			re.Entity.Type = "function"
			if x.Recv != nil && len(x.Recv.List) > 0 {
				re.Entity.Type = "method"
				re.ReceiverType = p.nodeToString(x.Recv.List[0].Type)
			}
			if x.Name != nil {
				re.Entity.Name = x.Name.Name
			}
			if x.Doc != nil {
				re.Entity.Description = strings.TrimSpace(x.Doc.Text())
			}

			sigJSON, paramCount, returnCount := p.extractFunctionSignature(x.Type)
			re.SignatureJSON = sigJSON
			re.ParamCount = paramCount
			re.ReturnCount = returnCount
			re.Entity.Signature = p.buildSignatureString(x.Type)

			if x.Body != nil {
				calls, localVars := p.extractCallsAndVars(x.Body, fset)
				re.Calls = calls
				re.LocalVariables = localVars
			}

		case *ast.GenDecl:
			if x.Tok == token.TYPE {
				for _, spec := range x.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						typeEntity := re
						entityIdentifiable = true
						if ts.Name != nil {
							typeEntity.Entity.Name = ts.Name.Name
						}
						if x.Doc != nil {
							typeEntity.Entity.Description = strings.TrimSpace(x.Doc.Text())
						} else if ts.Doc != nil {
							typeEntity.Entity.Description = strings.TrimSpace(ts.Doc.Text())
						}

						astMeta := make(map[string]interface{})
						fieldsOrMethods := []map[string]string{}

						switch typeSpecType := ts.Type.(type) {
						case *ast.StructType:
							typeEntity.Entity.Type = "struct"
							if typeSpecType.Fields != nil {
								for _, field := range typeSpecType.Fields.List {
									fieldTypeStr := p.nodeToString(field.Type)
									for _, nameId := range field.Names {
										fieldsOrMethods = append(fieldsOrMethods, map[string]string{"name": nameId.Name, "type": fieldTypeStr})
									}
									if len(field.Names) == 0 {
										fieldsOrMethods = append(fieldsOrMethods, map[string]string{"name": "", "type": fieldTypeStr})
									}
								}
							}
							astMeta["fields"] = fieldsOrMethods
						case *ast.InterfaceType:
							typeEntity.Entity.Type = "interface"
							if typeSpecType.Methods != nil {
								for _, method := range typeSpecType.Methods.List {
									if len(method.Names) > 0 {
										methodName := method.Names[0].Name

										if ft, ok := method.Type.(*ast.FuncType); ok {
											methodSigJSON, _, _ := p.extractFunctionSignature(ft)
											fieldsOrMethods = append(fieldsOrMethods, map[string]string{"name": methodName, "signature": methodSigJSON})
										} else {
											fieldsOrMethods = append(fieldsOrMethods, map[string]string{"name": methodName, "type": p.nodeToString(method.Type)})
										}
									}
								}
							}
							astMeta["methods"] = fieldsOrMethods
						default:
							typeEntity.Entity.Type = "type"
							typeEntity.Entity.Signature = p.nodeToString(ts.Type)
						}

						if len(astMeta) > 0 {
							metaBytes, _ := json.Marshal(astMeta)
							typeEntity.ASTMetadata = string(metaBytes)
						}

						typeEntity.Entity.LineStart = fset.Position(ts.Pos()).Line
						typeEntity.Entity.LineEnd = fset.Position(ts.End()).Line
						typeEntity.Entity.Content = p.extractNodeContent(content, ts.Pos(), ts.End(), fset)
						richEntities = append(richEntities, typeEntity)
					}
				}
				entityIdentifiable = false
			} else if x.Tok == token.VAR || x.Tok == token.CONST {
				for _, spec := range x.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							varEntity := re
							entityIdentifiable = true
							varEntity.Entity.Name = name.Name
							if x.Tok == token.VAR {
								varEntity.Entity.Type = "variable"
							} else {
								varEntity.Entity.Type = "constant"
							}
							if vs.Type != nil {
								varSig := make(map[string]string)
								varSig["type"] = p.nodeToString(vs.Type)
								sigBytes, _ := json.Marshal(varSig)
								varEntity.SignatureJSON = string(sigBytes)
								varEntity.Entity.Signature = p.nodeToString(vs.Type)
							}
							if x.Doc != nil {
								varEntity.Entity.Description = strings.TrimSpace(x.Doc.Text())
							} else if vs.Doc != nil {
								varEntity.Entity.Description = strings.TrimSpace(vs.Doc.Text())
							}

							varEntity.Entity.LineStart = fset.Position(vs.Pos()).Line
							varEntity.Entity.LineEnd = fset.Position(vs.End()).Line
							varEntity.Entity.Content = p.extractNodeContent(content, vs.Pos(), vs.End(), fset)
							richEntities = append(richEntities, varEntity)
						}
					}
				}
				entityIdentifiable = false
			}
		}

		if entityIdentifiable {
			re.Entity.LineStart = fset.Position(n.Pos()).Line
			re.Entity.LineEnd = fset.Position(n.End()).Line
			re.Entity.Content = p.extractNodeContent(content, n.Pos(), n.End(), fset)
			richEntities = append(richEntities, re)
		}
		return true
	})

	return richEntities, nil
}

func (p *GoParser) extractNodeContent(originalContent string, startPos, endPos token.Pos, fset *token.FileSet) string {
	startOffset := fset.Position(startPos).Offset
	endOffset := fset.Position(endPos).Offset

	contentBytes := []byte(originalContent)
	if startOffset < 0 || endOffset < 0 || endOffset < startOffset || endOffset > len(contentBytes) {
		return ""
	}
	return string(contentBytes[startOffset:endOffset])
}

func (p *GoParser) nodeToString(n ast.Node) string {
	var buf bytes.Buffer
	fset := token.NewFileSet()
	err := ast.Fprint(&buf, fset, n, ast.NotNilFilter)
	if err != nil {
		return "error_printing_node"
	}
	return strings.ReplaceAll(buf.String(), "\n", " ")
}

func (p *GoParser) buildSignatureString(funcType *ast.FuncType) string {
	if funcType == nil {
		return ""
	}
	var params []string
	if funcType.Params != nil {
		for _, field := range funcType.Params.List {
			typeName := p.nodeToString(field.Type)
			if len(field.Names) > 0 {
				for _, name := range field.Names {
					params = append(params, name.Name+" "+typeName)
				}
			} else {
				params = append(params, typeName)
			}
		}
	}
	paramStr := strings.Join(params, ", ")

	var results []string
	if funcType.Results != nil {
		for _, field := range funcType.Results.List {
			typeName := p.nodeToString(field.Type)
			if len(field.Names) > 0 {
				for _, name := range field.Names {
					results = append(results, name.Name+" "+typeName)
				}
			} else {
				results = append(results, typeName)
			}
		}
	}
	resultStr := strings.Join(results, ", ")
	if len(results) > 1 {
		resultStr = "(" + resultStr + ")"
	}
	if resultStr != "" {
		return "(" + paramStr + ") " + resultStr
	}
	return "(" + paramStr + ")"
}

func (p *GoParser) extractFunctionSignature(funcType *ast.FuncType) (string, int, int) {
	if funcType == nil {
		return "{}", 0, 0
	}

	var signature struct {
		Params  []map[string]string `json:"params,omitempty"`
		Returns []map[string]string `json:"returns,omitempty"`
	}
	var paramCount, returnCount int

	if funcType.Params != nil {
		for _, field := range funcType.Params.List {
			typeName := p.nodeToString(field.Type)
			numNames := len(field.Names)
			if numNames == 0 {
				paramCount++
				signature.Params = append(signature.Params, map[string]string{"type": typeName})
			} else {
				paramCount += numNames
				for _, name := range field.Names {
					signature.Params = append(signature.Params, map[string]string{"name": name.Name, "type": typeName})
				}
			}
		}
	}

	if funcType.Results != nil {
		for _, field := range funcType.Results.List {
			typeName := p.nodeToString(field.Type)
			numNames := len(field.Names)
			if numNames == 0 {
				returnCount++
				signature.Returns = append(signature.Returns, map[string]string{"type": typeName})
			} else {
				returnCount += numNames
				for _, name := range field.Names {
					signature.Returns = append(signature.Returns, map[string]string{"name": name.Name, "type": typeName})
				}
			}
		}
	}

	jsonBytes, _ := json.Marshal(signature)
	return string(jsonBytes), paramCount, returnCount
}

func (p *GoParser) extractCallsAndVars(body *ast.BlockStmt, fset *token.FileSet) ([]CallRelation, []string) {
	var calls []CallRelation
	var localVars []string

	if body == nil {
		return calls, localVars
	}

	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			callRel := CallRelation{LineNumber: fset.Position(x.Lparen).Line}
			var args []string
			for _, arg := range x.Args {
				args = append(args, p.nodeToString(arg))
			}
			callRel.Arguments = args

			switch fun := x.Fun.(type) {
			case *ast.Ident:
				callRel.TargetName = fun.Name
			case *ast.SelectorExpr:
				callRel.TargetName = fun.Sel.Name
				callRel.TargetContext = p.nodeToString(fun.X)
				callRel.IsCrossFileOrPackage = true
			default:
				callRel.TargetName = p.nodeToString(fun)

			}
			calls = append(calls, callRel)

		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, lhs := range x.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						isNewVar := true
						for _, v := range localVars {
							if v == ident.Name {
								isNewVar = false
								break
							}
						}
						if isNewVar {
							localVars = append(localVars, ident.Name)
						}
					}
				}
			}
		case *ast.DeclStmt:
			if genDecl, ok := x.Decl.(*ast.GenDecl); ok {
				if genDecl.Tok == token.VAR {
					for _, spec := range genDecl.Specs {
						if valueSpec, ok := spec.(*ast.ValueSpec); ok {
							for _, name := range valueSpec.Names {
								localVars = append(localVars, name.Name)
							}
						}
					}
				}
			}
		}
		return true
	})
	return calls, localVars
}
