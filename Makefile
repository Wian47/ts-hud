.PHONY: build test vet run clean

build:
	CGO_ENABLED=0 go build -o ts-hud .

test:
	go test ./...

vet:
	go vet ./...

run: build
	./ts-hud

clean:
	rm -f ts-hud
