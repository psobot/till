default: $(wildcard src/*.go)
	mkdir -p bin
	go build -o bin/tilld $(wildcard src/*.go) 

run: default
	./bin/tilld

test: default test/runner.py
	python test/runner.py