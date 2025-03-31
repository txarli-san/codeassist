.PHONY: build run scan query interactive stats clean

BINARY_NAME=codeassist

build:
	go build -o bin/$(BINARY_NAME) cmd/codeassist/main.go

run:
	go run cmd/codeassist/main.go

scan:
	go run cmd/codeassist/main.go scan $(filter-out $@,$(MAKECMDGOALS))

query:
	go run cmd/codeassist/main.go query $(filter-out $@,$(MAKECMDGOALS))

interactive:
	go run cmd/codeassist/main.go interactive

stats:
	go run cmd/codeassist/main.go stats

clean:
	go clean
	rm -f bin/$(BINARY_NAME)
