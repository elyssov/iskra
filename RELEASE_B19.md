## Iskra v0.6.0-alpha — Build #19 "Cutty Sark"

*Named after the legendary tea clipper Cutty Sark (1869) — the fastest ship of her era, she carried cargo across oceans in sealed holds. Like our encrypted messages crossing the mesh.*

---

### Передача файлов / File Transfer (NEW)

Файлы теперь можно отправлять через Искру. Зашифрованно, по частям, через mesh.

- Кнопка-скрепка в чате — прикрепить файл
- Файл сжимается (zip), нарезается на чанки по 900 КБ
- Каждый чанк — отдельное зашифрованное сообщение с уникальным ID
- Получатель собирает чанки (порядок не важен) и распаковывает
- **Лимит: 10 МБ** — документы, фото, короткое видео
- **TTL чанков: 7 дней** (короче чем у текста — не засоряет трюмы)
- **HopTTL: 5** (меньше хопов чем текст — файлы не расползаются по всей сети)
- Перехватчик видит кучу непонятных блобов — не знает что они части одного файла

Files can now be sent through Iskra. Encrypted, chunked, via mesh.

- Paperclip button in chat — attach file
- File compressed (zip), split into 900KB chunks
- Each chunk = separate encrypted message with unique ID
- Receiver assembles chunks (order doesn't matter) and decompresses
- **Limit: 10 MB** — documents, photos, short video
- **Chunk TTL: 7 days** (shorter than text — doesn't clog holds)
- **HopTTL: 5** (fewer hops than text — files don't spread across entire mesh)
- Interceptor sees random blobs — can't tell they're parts of one file

---

### Также в этом билде / Also in this build

- **Polling оптимизирован** — 2с messages, 5с unread, 10с contacts. Пропуск во время набора текста
- **Ники не слетают** — Master login не ломает contacts VaultKey
- **Hold sync cooldown** — максимум раз в 30 секунд
- **Unread для всех** — сообщения от незнакомцев тоже показывают badge
- **Relay deadlock починен** — broadcast вне lock, write deadline 5с
- **Discovery cooldown** — 2 минуты на peer (вместо спама каждые 5 сек)

---

**Files:**
- `iskra-build19.apk` — Android (26 MB)
- `iskra-build19.exe` — Windows (11 MB)
- `iskra-linux` — Linux binary
- `relay-linux` — relay server

**Feedback:** elyssov@gmail.com or via "Master" account in Iskra

*"A spark will kindle a flame" / "Cutty Sark carried tea — we carry freedom"*
