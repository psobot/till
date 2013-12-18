bin/tilld: $(wildcard src/*.go)
	mkdir -p bin
	go build -o bin/tilld $(wildcard src/*.go)

run: bin/tilld
	./bin/tilld

test: bin/tilld test/runner.py
	python test/runner.py