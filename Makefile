.PHONY: all test build relay clean

# Тесты
test:
	go test ./internal/... -v -count=1

# Go бинарник (для разработки/тестирования)
build:
	go build -ldflags="-s -w" -o dist/iskra ./cmd/iskra/

# Relay сервер
relay:
	go build -ldflags="-s -w" -o dist/relay ./cmd/relay/

# Linux builds
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/iskra-linux ./cmd/iskra/

relay-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/relay-linux ./cmd/relay/

# Android .aar через gomobile
build-aar:
	gomobile bind -target=android -o android/app/libs/iskra.aar ./cmd/iskra-mobile/

# Очистка
clean:
	rm -rf dist/
