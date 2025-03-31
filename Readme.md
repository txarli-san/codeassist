# CodeAssist

CodeAssist is a CLI tool that helps you understand and navigate your codebase by indexing your code and providing intelligent answers to your questions using LLMs (Large Language Models).

## Features

- **Code Indexing**: Recursively scans your codebase to extract functions, methods, classes, and other code structures
- **Intelligent Search**: Finds relevant code based on your natural language queries
- **Context-Aware Responses**: Uses the extracted code context to generate accurate and helpful answers
- **Multiple Language Support**: Includes parsers for Go, Ruby, and JavaScript
- **SQLite Storage**: All indexed code is stored in a local SQLite database for fast querying
- **Interactive Mode**: Ask questions about your codebase in a conversational interface

## Prerequisites

- Go 1.18 or higher
- [Ollama](https://ollama.ai/) installed and running locally (or remotely accessible)
- LLM model like `codellama` pulled in Ollama

## Installation

1. Clone this repository:
   ```
   git clone https://github.com/txarli-san/codeassist.git
   cd codeassist
   ```

2. Build the binary:
   ```
   go build -o codeassist
   ```

3. Make sure Ollama is running with the required model (e.g., `codellama`):
   ```
   ollama pull codellama
   ```

## Usage

CodeAssist has several subcommands:

### Scanning Your Codebase

Before you can query your codebase, you need to scan and index it:

```
./codeassist scan -path=/path/to/your/project -langs=go,rb,js
```

Available options:
- `-path`: Directory to scan (default: ".")
- `-langs`: Comma-separated list of languages to scan (default: "go,rb,js")

### Querying Your Codebase

Once your codebase is indexed, you can ask questions about it:

```
./codeassist query -q="How does the database initialization work?"
```

Available options:
- `-q`: The question to ask about your codebase

### Interactive Mode

For multiple queries in succession, use the interactive mode:

```
./codeassist interactive
```

This starts a prompt where you can type questions and get responses in real-time.

### Viewing Database Statistics

To see statistics about the indexed codebase:

```
./codeassist stats
```

This will show information like:
- Total number of files indexed
- Total number of entities (functions, methods, classes, etc.) extracted
- Breakdown of entity types
- Languages distribution

### Global Options

These options can be used with any subcommand:

- `-db`: Path to the SQLite database file (default: "codeassist.db")
- `-ollama-url`: URL for Ollama API (default: "http://localhost:11434")
- `-model`: Ollama model to use (default: "codellama")

Example:
```
./codeassist -model=llama2 -ollama-url=http://my-ollama-server:11434 query -q="How does the API work?"
```

## How It Works

1. The `scan` command walks through your codebase, parsing files according to their language
2. Code structures (functions, methods, classes) are extracted using language-specific parsers
3. The extracted entities and whole files are stored in an SQLite database
4. When you query, CodeAssist finds the most relevant code sections related to your query
5. The relevant code is sent to the LLM along with your question to generate a contextually aware response

## Architecture

CodeAssist consists of several key components:

- **Scanner**: Recursively scans directories and passes files to appropriate parsers
- **Parsers**: Language-specific parsers for Go, JavaScript, and Ruby that extract code structures
- **Storage**: SQLite database that stores files, entities, and provides search functionality
- **LLM Client**: Connects to Ollama to generate responses based on code context

## Supported Languages

- Go: Functions, methods, structs, interfaces, and packages
- JavaScript: Functions, arrow functions, classes, and methods
- Ruby: Classes, modules, and methods

## Limitations

- The FTS (full-text search) feature requires SQLite to be compiled with FTS4 support
- The quality of responses depends on the underlying LLM model
- The tool currently only analyzes syntactic structure, not semantic relationships between code elements

## Advanced Usage

### Using a Different Model

CodeAssist works with any model available in your Ollama installation:

```
./codeassist -model=mistral query -q="How does error handling work in this codebase?"
```

### Customizing Database Location

You can change where the database is stored:

```
./codeassist -db=/path/to/database.db scan -path=/path/to/project
```

### Using a Remote Ollama Instance

If you have Ollama running on a different machine:

```
./codeassist -ollama-url=http://ollama-server:11434 interactive
```

## Troubleshooting

- **"Error connecting to Ollama"**: Ensure Ollama is running and accessible at the specified URL
- **"Model not found"**: Make sure you've pulled the model you're trying to use (`ollama pull modelname`)
- **Slow responses**: Try using a smaller or more efficient model, or limit the scan to only necessary directories
- **Parser errors**: Some complex language constructs might not be properly parsed. Please report any issues

## Contributing

Contributions are welcome! Some areas that could use improvement:

1. Additional language parsers (Python, TypeScript, etc.)
2. More sophisticated code analysis
3. UI improvements
4. Performance optimizations

## License

LoL nope... do whatever, ask for nothing.
