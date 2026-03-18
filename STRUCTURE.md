# ИСКРА — Структура проекта

---

## Go-проект

```
iskra/
├── cmd/
│   ├── iskra/
│   │   └── main.go              # Entry point: CLI + daemon
│   └── relay/
│       └── main.go              # Минимальный relay-сервер (отдельный бинарник)
│
├── internal/
│   ├── identity/
│   │   ├── keys.go              # Ed25519/X25519 генерация из seed
│   │   ├── keys_test.go
│   │   ├── mnemonic.go          # Seed ↔ 24 русских слова
│   │   ├── mnemonic_test.go
│   │   └── base58.go            # Base58 encode/decode
│   │
│   ├── crypto/
│   │   ├── encrypt.go           # XSalsa20-Poly1305 + обфускация
│   │   ├── encrypt_test.go
│   │   ├── sign.go              # Ed25519 подпись/верификация
│   │   ├── sign_test.go
│   │   ├── pow.go               # Hashcash PoW
│   │   └── pow_test.go
│   │
│   ├── message/
│   │   ├── message.go           # Структура Message
│   │   ├── serialize.go         # Бинарная сериализация/десериализация
│   │   ├── serialize_test.go
│   │   └── types.go             # ContentType константы
│   │
│   ├── mesh/
│   │   ├── discovery.go         # LAN multicast beacon
│   │   ├── discovery_test.go
│   │   ├── peer.go              # Peer list management
│   │   ├── transport.go         # KCP sessions
│   │   ├── sync.go              # HAVE/WANT/MSG протокол обмена трюмами
│   │   └── wifidirect.go        # Wi-Fi Direct (заглушка, если не успеем)
│   │
│   ├── store/
│   │   ├── hold.go              # Трюм (файловое хранение)
│   │   ├── hold_test.go
│   │   ├── inbox.go             # Входящие (расшифрованные)
│   │   ├── bloom.go             # Bloom filter дедупликация
│   │   └── contacts.go          # Хранение контактов
│   │
│   ├── update/
│   │   └── updater.go           # Проверка и применение обновлений
│   │
│   └── web/
│       ├── server.go            # HTTP-сервер на localhost
│       ├── api.go               # REST API handlers
│       └── static/              # Встраиваемые файлы (go:embed)
│           ├── index.html       # Единственная HTML-страница
│           ├── app.js           # Вся логика интерфейса
│           └── style.css        # Стили
│
├── wordlist/
│   └── russian.go               # 256 русских слов для мнемоники
│
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Зависимости (go.mod)

```
module github.com/iskra-messenger/iskra

go 1.22

require (
    github.com/xtaci/kcp-go/v5 v5.6.8
    github.com/bits-and-blooms/bloom/v3 v3.7.0
    golang.org/x/crypto v0.21.0
)
```

Минимум зависимостей. Только то, что нельзя написать за день:
- KCP — reliable UDP (сложный протокол, не переписываем)
- Bloom filter — математически выверенная структура
- golang.org/x/crypto — стандартная крипто-библиотека Go

Всё остальное — пишем сами: base58, сериализация, HTTP-сервер (стандартный `net/http`), мнемоника.

---

## Android-обёртка

```
android/
├── app/
│   ├── src/main/
│   │   ├── java/com/iskra/app/
│   │   │   └── MainActivity.kt    # WebView + запуск Go-сервиса
│   │   ├── AndroidManifest.xml     # Разрешения
│   │   └── res/
│   │       └── ...
│   └── libs/
│       └── iskra.aar              # Go-ядро (gomobile)
├── build.gradle
└── settings.gradle
```

**Разрешения (AndroidManifest.xml):**
```xml
<uses-permission android:name="android.permission.INTERNET" />
<uses-permission android:name="android.permission.ACCESS_WIFI_STATE" />
<uses-permission android:name="android.permission.CHANGE_WIFI_STATE" />
<uses-permission android:name="android.permission.ACCESS_NETWORK_STATE" />
<uses-permission android:name="android.permission.CHANGE_NETWORK_STATE" />
<uses-permission android:name="android.permission.ACCESS_FINE_LOCATION" />
<!-- Wi-Fi Direct требует location permission на Android -->
```

**MainActivity.kt — минимальная:**
```kotlin
class MainActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Запустить Go-ядро
        Iskra.start(filesDir.absolutePath, 0) // 0 = random port
        val port = Iskra.getPort()
        // Показать WebView
        val webView = WebView(this)
        webView.settings.javaScriptEnabled = true
        webView.loadUrl("http://localhost:$port")
        setContentView(webView)
    }
}
```

---

## Makefile

```makefile
.PHONY: all test build-linux build-android relay clean

# Тесты
test:
	go test ./internal/... -v -count=1

# Go бинарник (для разработки/тестирования на компе)
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/iskra ./cmd/iskra/

# Relay сервер
relay:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/relay ./cmd/relay/

# Android .aar через gomobile
build-aar:
	gomobile bind -target=android -o android/app/libs/iskra.aar ./cmd/iskra-mobile/

# Android APK (требует Android SDK)
build-android: build-aar
	cd android && ./gradlew assembleRelease

# Очистка
clean:
	rm -rf dist/
	rm -f android/app/libs/iskra.aar
```

---

## Relay (отдельный бинарник)

```go
// cmd/relay/main.go — ВЕСЬ код relay
// ~100-150 строк. WebSocket. Без зависимостей кроме gorilla/websocket.
// 
// Логика:
// 1. Клиент подключается по WebSocket, отправляет свой pubkey (20 байт UserID)
// 2. Relay хранит map: UserID → *websocket.Conn
// 3. Клиент отправляет сообщение → relay смотрит RecipientID → если онлайн, передаёт
// 4. Если оффлайн → хранит в памяти ([]Message), отдаёт при подключении получателя
// 5. НЕ логирует. НЕ расшифровывает. НЕ хранит на диск.
```
