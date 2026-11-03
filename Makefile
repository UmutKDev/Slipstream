VERSION ?= 0.1.0

.PHONY: test engine app run clean

test:
	go vet ./...
	go test ./...

# Bu Mac için motoru derler (geliştirme).
engine:
	go build -o bin/slipstreamd ./cmd/slipstreamd

app:
	VERSION=$(VERSION) bash ./scripts/build-app.sh

# Motoru uygulama olmadan, sistem proxy'si açık biçimde çalıştırır.
run: engine
	./bin/slipstreamd run -system-proxy

clean:
	rm -rf bin build macos/.build
