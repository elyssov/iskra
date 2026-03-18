# ИСКРА — Архитектура (альфа v0.1)

---

## 1. Криптография

### 1.1 Ключи

При первом запуске генерируется **seed** — 32 случайных байта (`crypto/rand`).

Из seed выводятся:
- **Ed25519 keypair** — для подписи (signing). `golang.org/x/crypto/ed25519`
- **X25519 keypair** — для шифрования (DH key exchange). Выводится из того же seed через `crypto/sha512` + clamping по RFC 7748.

**User ID** = первые 20 символов base58(Ed25519 public key). Уникальность гарантирована криптографически (117 бит энтропии, коллизия при 10⁹ пользователей: p ≈ 10⁻¹⁷).

**Мнемоника** = seed закодированный в 24 русских слова из словаря в 256 слов (8 бит на слово × 24 = 192 бита; оставшиеся 64 бита seed используются как checksum).

### 1.2 Шифрование сообщений

**Шаг 1 — Key Agreement:**
```
ephemeral_keypair = X25519.generate()
shared_secret = X25519(ephemeral_private, recipient_public)
symmetric_key = HMAC-SHA256(shared_secret, "iskra-v1-message-key")
```

**Шаг 2 — Шифрование:**
```
nonce = random(24 bytes)
ciphertext = XSalsa20-Poly1305.encrypt(symmetric_key, nonce, plaintext)
```

**Шаг 3 — Обфускация (поверх libsodium):**
```
obfuscation_stream = HMAC-SHA256(shared_secret, nonce || "iskra-obf") → расширить через повторные HMAC до длины ciphertext
obfuscated = ciphertext XOR obfuscation_stream
```

Зачем: выходной поток не похож на стандартный libsodium output. DPI или реверс-инженер не узнает NaCl по сигнатуре. Под обфускацией — настоящий XSalsa20, который не ломается.

**Шаг 4 — Подпись:**
```
signature = Ed25519.sign(author_private_key, message_header || obfuscated_ciphertext)
```

**Финальный пакет:**
```
[ephemeral_public_key (32)] [nonce (24)] [obfuscated_ciphertext (variable)] [signature (64)]
```

### 1.3 Расшифровка

```
shared_secret = X25519(recipient_private, ephemeral_public)
symmetric_key = HMAC-SHA256(shared_secret, "iskra-v1-message-key")
obfuscation_stream = HMAC-SHA256(shared_secret, nonce || "iskra-obf") → расширить
ciphertext = obfuscated XOR obfuscation_stream
plaintext = XSalsa20-Poly1305.decrypt(symmetric_key, nonce, ciphertext)
// Проверить signature через Ed25519.verify(author_public_key, ...)
```

### 1.4 Proof-of-Work (антиспам)

Hashcash-подобный. Перед отправкой:
```
pow_input = message_id || timestamp || nonce
pow_hash = SHA256(pow_input)
// Искать nonce пока первые N бит pow_hash не будут нулями
// Для текста: N=16 (~0.1 сек)
// Для будущих файлов: N=24 (~30 сек)
```

Получатель и промежуточные ноды проверяют PoW перед обработкой.

---

## 2. Структура сообщения

```go
type Message struct {
    // Заголовок (не шифруется, нужен для маршрутизации)
    Version     uint8     // Версия протокола (1)
    ID          [32]byte  // SHA256(всё остальное)
    RecipientID [20]byte  // Первые 20 байт pubkey получателя
    TTL         uint32    // Секунды до истечения (default 1209600 = 14 дней)
    Timestamp   int64     // Unix время создания
    ContentType uint8     // 0=text, 1=delivery_confirm, 2=contacts_sync, 255=app_update
    
    // Криптоблок (шифруется)
    EphemeralPub [32]byte // X25519 ephemeral public key
    Nonce        [24]byte // XSalsa20 nonce
    Payload      []byte   // Обфусцированный ciphertext
    
    // Подпись
    AuthorPub   [32]byte  // Ed25519 pubkey автора
    Signature   [64]byte  // Ed25519 подпись
    
    // PoW
    PoWNonce    uint64
}
```

**Сериализация:** простой бинарный формат. Не protobuf, не JSON — минимальный overhead, максимальная компактность. Формат:
```
[version:1][id:32][recipientID:20][ttl:4][timestamp:8][contentType:1]
[ephemeralPub:32][nonce:24][payloadLen:4][payload:variable]
[authorPub:32][signature:64][powNonce:8]
```

Все числа — big-endian.

---

## 3. Discovery

### 3.1 LAN Multicast

**Адрес:** 239.42.42.42 **Порт:** 4242 **Интервал:** 60 секунд

**Beacon формат:**
```
[magic: "ISKRA1" (6 bytes)]
[pubkey: Ed25519 public key (32 bytes)]
[listen_port: uint16 (2 bytes)]
[version: uint8 (1 byte)]
[timestamp: int64 (8 bytes)]
```

При получении beacon от неизвестной ноды:
1. Добавить в peer list: {pubkey, IP (из UDP source), listen_port, last_seen}
2. Инициировать handshake (обмен ID)
3. Начать синхронизацию трюмов (отдать то, чего у другой ноды нет)

### 3.2 Wi-Fi Direct (если реализуемо в альфе)

Android Wi-Fi P2P API (`android.net.wifi.p2p`):
1. `discoverPeers()` раз в 60 секунд
2. При обнаружении — `requestConnect()`
3. После соединения — та же логика что LAN: handshake + sync

Если Wi-Fi Direct сложно в альфе — пропустить, реализовать в v0.2. LAN multicast — приоритет.

---

## 4. Транспорт

### 4.1 KCP (Reliable UDP)

Библиотека: `github.com/xtaci/kcp-go/v5`

Каждое соединение между двумя нодами — KCP-сессия. Поверх — простой протокол:

```
Handshake:
  → HELLO [my_pubkey (32)] [my_listen_port (2)]
  ← HELLO [their_pubkey (32)] [their_listen_port (2)]

Sync (обмен трюмами):
  → HAVE [bloom_filter (variable)]  // "У меня есть сообщения с этими ID"
  ← WANT [list of message IDs]       // "Дай мне вот эти"
  → MSG [serialized Message]          // Отправка каждого запрошенного
  ← ACK [message_id (32)]            // Подтверждение получения

Direct message:
  → MSG [serialized Message]
  ← ACK [message_id (32)]
```

### 4.2 Fallback: простой relay

Для альфы — один Go-бинарник на VPS. Минимальный WebSocket relay:
- Клиент подключается, отправляет свой pubkey
- Relay хранит mapping: pubkey → websocket connection
- Клиент отправляет сообщение → relay передаёт по recipientID
- Если получатель оффлайн → relay хранит в памяти (до перезапуска; это костыль)

Relay видит ТОЛЬКО зашифрованные + обфусцированные блобы. Не знает содержимого, не может расшифровать.

Код relay — максимально простой. 100-150 строк. Чтобы любой мог поднять свой за 5 минут.

---

## 5. Мешок почты (Store-and-Forward)

### 5.1 Трюм

Директория на устройстве: `~/.iskra/hold/`
Каждое сообщение — отдельный файл: `{message_id_hex}.msg`

При получении сообщения:
1. Проверить подпись
2. Проверить PoW
3. Проверить Bloom filter (видели ли уже)
4. Если message для нас → расшифровать, показать пользователю, отправить delivery_confirm
5. Если message НЕ для нас → положить в трюм
6. При контакте с другой нодой → обменяться (protocol HAVE/WANT/MSG выше)

### 5.2 Bloom Filter

Библиотека: `github.com/bits-and-blooms/bloom/v3`

Параметры: 1M expected items, 0.1% false positive rate ≈ 1.8 МБ.
Хранит ID всех сообщений, которые нода видела (и в трюме, и уже доставленных, и удалённых).

### 5.3 Delivery Confirm

Когда получатель расшифровал сообщение — он создаёт `delivery_confirm`:
```
ContentType = 1
Payload = [original_message_id (32 bytes)]
RecipientID = [all zeros — broadcast]
```

delivery_confirm распространяется по mesh. Каждая нода, получив его, удаляет соответствующее сообщение из трюма.

### 5.4 TTL (заложен, не активирован)

Поле TTL в структуре Message заполняется (default = 14 дней). Но автоудаление по TTL в альфе не реализовано. В v0.2 — при каждой синхронизации проверять: если timestamp + TTL < now → удалить из трюма.

---

## 6. Механизм обновления (ползучий апдейт)

Ключ разработчика (Ed25519 pubkey) — захардкожен в клиенте.

Обновление = специальное сообщение:
```
ContentType = 255 (app_update)
RecipientID = [all zeros — broadcast]
AuthorPub = [developer pubkey]
Payload = [version (uint32)] [apk_sha256 (32)] [apk_url (string)] [changelog (string)]
```

Клиент, получив обновление:
1. Проверить подпись (только от developer key!)
2. Если version > текущей → показать пользователю "Доступно обновление"
3. Пользователь нажимает → скачать APK (по URL или в будущем через mesh)
4. Проверить SHA256 скачанного файла
5. Предложить установку (Android intent)

В альфе: URL для скачивания. В v0.3: APK нарезается на чанки и распространяется через mesh.

---

## 7. Хранение контактов

**Контакт:**
```go
type Contact struct {
    Name      string   // Имя (введённое пользователем)
    PubKey    [32]byte // Ed25519 public key
    UserID    string   // Первые 20 символов base58(PubKey)
    AddedAt   int64    // Когда добавлен
    LastSeen  int64    // Когда последний раз видели в сети
}
```

**Хранение:** JSON-файл `~/.iskra/contacts.json`, зашифрованный ключом, выведенным из seed пользователя.

**Импорт из Искра-Мост:** JSON-файл с массивом `{name, publicKey}`. Парсим, добавляем в контакты.

---

## 8. UI (минимальный)

Встроенный HTTP-сервер на Go (localhost:PORT). Интерфейс — одна HTML-страница с JS.

**Экраны:**

1. **Первый запуск:** генерация ключей, показать ID + мнемонику, предложить сохранить.
2. **Главный:** список контактов (имя, ID, статус: online/offline/last seen). Кнопка "+" — добавить контакт по ID или QR.
3. **Чат:** сообщения с конкретным контактом. Поле ввода, кнопка "отправить". Статус доставки: ✓ отправлено, ✓✓ доставлено.
4. **Статус:** режим работы (LAN / relay / offline), количество пиров, размер трюма.

**Стиль:** Telegram-like. Светлый фон, синие пузыри отправленных, белые — полученных. Минимально, но узнаваемо.

**REST API (Go → JS):**
```
GET  /api/contacts          → список контактов
POST /api/contacts          → добавить контакт {name, pubkey}
GET  /api/messages/:pubkey  → история с контактом
POST /api/messages/:pubkey  → отправить сообщение {text}
GET  /api/status            → статус ноды
GET  /api/identity          → мой ID, pubkey, мнемоника
POST /api/import            → импорт контактов из JSON
```
