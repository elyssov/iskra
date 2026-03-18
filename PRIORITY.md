# ИСКРА — Порядок реализации

## Каждый шаг = рабочий, тестируемый результат

---

## Шаг 1: identity/ (день 1)

**Цель:** генерация ключей, base58, мнемоника.

**Файлы:** `internal/identity/keys.go`, `base58.go`, `mnemonic.go` + тесты.
**Также:** `wordlist/russian.go` (словарь — см. WORDLIST.md)

**Что должно работать после шага:**
```go
seed := identity.GenerateSeed()          // 32 случайных байта
kp := identity.KeypairFromSeed(seed)     // Ed25519 + X25519
userID := identity.UserID(kp.Ed25519Pub) // 20 символов base58
words := identity.SeedToMnemonic(seed)   // 24 русских слова
seed2 := identity.MnemonicToSeed(words)  // Обратно (должен совпасть)
b58 := identity.ToBase58(kp.Ed25519Pub)  // Полный ключ в base58
```

**Тесты:**
- Генерация → мнемоника → обратно = тот же seed
- Два вызова GenerateSeed() дают разные результаты
- UserID длиной ровно 20 символов
- Base58 roundtrip (encode → decode = original)

---

## Шаг 2: crypto/ (день 2)

**Цель:** шифрование, расшифровка, подпись, PoW.

**Файлы:** `internal/crypto/encrypt.go`, `sign.go`, `pow.go` + тесты.

**Что должно работать:**
```go
// Шифрование
ct := crypto.Encrypt(senderKeypair, recipientPubKey, plaintext)
// Расшифровка  
pt, err := crypto.Decrypt(recipientKeypair, ct)
// pt == plaintext

// Подпись
sig := crypto.Sign(keypair.Ed25519Private, data)
ok := crypto.Verify(keypair.Ed25519Pub, data, sig)

// PoW
nonce := crypto.SolvePoW(messageID, timestamp, difficulty)
ok := crypto.VerifyPoW(messageID, timestamp, nonce, difficulty)
```

**Тесты:**
- Encrypt → Decrypt = оригинал
- Decrypt чужим ключом → ошибка
- Подпись верифицируется правильным ключом
- Подпись НЕ верифицируется неправильным ключом
- Тамперинг ciphertext → ошибка расшифровки (Poly1305 ловит)
- PoW: решение проверяется, неправильный nonce отклоняется
- Обфускация: зашифрованные данные НЕ содержат узнаваемых паттернов NaCl

---

## Шаг 3: message/ (день 3)

**Цель:** структура сообщения, сериализация.

**Файлы:** `internal/message/message.go`, `serialize.go`, `types.go` + тесты.

**Что должно работать:**
```go
msg := message.New(authorKP, recipientPubKey, "Привет из Искры")
raw := msg.Serialize()       // []byte
msg2 := message.Deserialize(raw)
// msg2.ID == msg.ID, msg2.Payload дешифруется в "Привет из Искры"
```

**Тесты:**
- Serialize → Deserialize = идентичный Message
- Корректный ID (SHA256 всех полей)
- Проверка подписи после десериализации
- Некорректные данные → ошибка десериализации

---

## Шаг 4: store/ (день 4)

**Цель:** трюм, bloom filter, хранение контактов.

**Файлы:** `internal/store/hold.go`, `bloom.go`, `contacts.go`, `inbox.go` + тесты.

**Что должно работать:**
```go
hold := store.NewHold("/path/to/hold")
hold.Store(msg)                    // Положить в трюм
msgs := hold.GetAll()              // Получить всё из трюма
hold.Delete(msgID)                 // Удалить (при delivery_confirm)

bloom := store.NewBloom()
bloom.Add(msgID)
seen := bloom.Contains(msgID)     // true

contacts := store.NewContacts("/path")
contacts.Add("Алиса", alicePubKey)
contacts.Import("/path/to/bridge-export.json")
list := contacts.List()
```

**Тесты:**
- Store → GetAll содержит сообщение
- Delete → GetAll не содержит
- Bloom: добавленный ID → Contains=true, не добавленный → false
- Contacts: Add, List, Import из JSON

---

## Шаг 5: mesh/discovery + mesh/transport (дни 5-6)

**Цель:** LAN discovery + KCP соединение + обмен трюмами.

**Файлы:** `internal/mesh/discovery.go`, `transport.go`, `peer.go`, `sync.go` + тесты.

**Что должно работать:**
```
Нода A запускается → шлёт multicast beacon каждые 60 сек
Нода B запускается → шлёт beacon → получает beacon от A
B добавляет A в peers → инициирует KCP-соединение
Handshake: обмен pubkey
Sync: A отправляет HAVE (bloom) → B отвечает WANT → A отправляет MSG
```

**Тест (интеграционный):**
- Запустить две ноды на localhost (разные порты)
- Нода A кладёт сообщение в трюм
- Ноды находят друг друга через multicast (на localhost — через loopback)
- Синхронизация → сообщение появляется в трюме ноды B

---

## Шаг 6: web/ (дни 7-9)

**Цель:** HTTP-сервер, REST API, минимальный UI.

**Файлы:** `internal/web/server.go`, `api.go`, `static/index.html`, `static/app.js`, `static/style.css`

**API:**
```
GET  /api/identity           → {userID, pubkey, mnemonic}
GET  /api/contacts           → [{name, pubkey, userID, lastSeen}]
POST /api/contacts           → добавить {name, pubkeyBase58}
GET  /api/messages/:userID   → [{id, from, text, timestamp, status}]
POST /api/messages/:userID   → отправить {text}
GET  /api/status             → {mode, peers, holdSize, version}
POST /api/import             → импорт контактов из JSON
```

**UI (static/index.html):**
Один файл. HTML + CSS + JS inline. Telegram-like:
- Левая панель: список контактов с последним сообщением
- Правая панель: чат с выбранным контактом
- На мобильном: одна панель, переключение тапом
- Сверху: ID пользователя, статус сети
- Стиль: светлый фон, синие/белые пузыри, минимализм

**Примечание для Claude Code:** UI не должен быть красивым. Он должен быть *функциональным*. Если стоит выбор между красотой и скоростью разработки — выбирай скорость. Красоту накрутим потом.

---

## Шаг 7: cmd/iskra + cmd/relay (дни 9-10)

**Цель:** собрать всё вместе. CLI демон + relay.

**cmd/iskra/main.go:**
```
iskra                    → запустить демон + открыть UI в браузере
iskra --port 8080        → указать порт для UI
iskra --debug            → включить логи
iskra --data /path       → указать директорию данных
```

При запуске:
1. Загрузить или создать keypair
2. Запустить mesh discovery
3. Запустить KCP listener
4. Запустить HTTP-сервер
5. Подключиться к relay (если указан: --relay ws://host:port)
6. Открыть http://localhost:PORT в браузере

**cmd/relay/main.go:**
```
relay                    → запустить на :8443
relay --port 9000        → указать порт
```

---

## Шаг 8: интеграция и тесты (дни 11-12)

**Цель:** два инстанса на одном компе обмениваются сообщениями.

**Тест-сценарий:**
1. Запустить `iskra --port 8080 --data /tmp/iskra-a`
2. Запустить `iskra --port 8081 --data /tmp/iskra-b`
3. В UI ноды A: добавить контакт (pubkey ноды B)
4. Отправить сообщение
5. В UI ноды B: сообщение появилось
6. Ответить
7. В UI ноды A: ответ появился

Также:
- Запустить relay, подключить обе ноды → сообщения через relay
- Остановить ноду B → отправить сообщение → запустить B → сообщение доставлено из трюма

---

## Шаг 9: Android APK (дни 13-14)

**Цель:** рабочий APK.

1. `gomobile bind` → iskra.aar
2. Минимальная Kotlin-обёртка (MainActivity с WebView)
3. Собрать APK
4. Установить на телефон
5. Проверить: генерация ключей, UI, отправка сообщения (через relay или LAN)

**Если gomobile капризничает** (а он может):
- Альтернатива: скомпилировать Go как исполняемый бинарник для android/arm64
- Запускать его из Kotlin как Process
- WebView → localhost как обычно

---

## Шаг 10: финальная сборка (день 14)

**Цель:** релизный APK + relay + документация.

1. Почистить код
2. Убрать debug-вывод
3. Проверить что --debug по умолчанию ВЫКЛЮЧЕН
4. Собрать release APK (минимизировать)
5. Собрать relay для linux/amd64
6. Написать README: как установить, как использовать, как поднять relay
7. Захардкодить developer pubkey для механизма обновлений

---

## Свобода действий

Если ты видишь что какой-то шаг можно сделать быстрее или лучше — делай. Если шаг занимает дольше ожидаемого — пропусти некритичное (Wi-Fi Direct, например) и двигайся дальше. Главное к концу дня 14:

**Два телефона обмениваются зашифрованными сообщениями. Без сервера (relay — временный костыль). С трюмом. С обнаружением в LAN. С UI, в котором можно набрать и отправить сообщение.**

Всё остальное — бонус.

🔥
