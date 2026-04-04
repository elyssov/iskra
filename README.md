# Iskra / Искра 2.0 "Восток"

**Peer-to-peer encrypted messenger that works when the internet doesn't.**

*"Поехали!" — Yuri Gagarin, April 12, 1961*

Iskra (Russian: *Искра* — "spark") is a censorship-resistant messenger built for environments where centralized infrastructure is compromised, blocked, or shut down. Every device running Iskra is both a client and a relay node — there is no central server to seize or block.

**What's new in 2.0:**
- Three-tab interface: Contacts / Chats / Mail
- Mail system with 60-day TTL for reliable delivery
- Dark theme
- Context menu on contacts (message, letter, QR, forward, rename, delete)
- Cosmic splash screen (because we're "Vostok")
- TTL by content type: chat 15 days, mail 60 days, channels 30 days
- File chunks excluded from store-and-forward (fixes hold bloat)
- Settings page with display name, theme, PIN management
- Built-in Lara contact (AI team member) with gold badge

> *"A spark will kindle a flame"*

---

## How It Works

Iskra uses a **store-and-forward mesh** inspired by 18th-century postal ships:

1. Your message is **encrypted end-to-end** on your device
2. It's stored in a local **hold** (encrypted cargo bay)
3. When your device encounters another Iskra node — via LAN, Wi-Fi Direct, or a relay — they **exchange holds**
4. Each node carries encrypted messages for others, delivering them when paths cross
5. Eventually, the message reaches its recipient and they **decrypt it locally**

No node except the intended recipient can read the message. Intermediate nodes are blind couriers.

```
  You ──encrypt──▶ [hold] ──mesh──▶ [hold] ──mesh──▶ [hold] ──decrypt──▶ Recipient
                     │                │                │
                 your phone      stranger's        friend's
                                  phone             phone
```

## Features

- **End-to-end encryption** — XSalsa20-Poly1305 + X25519 key exchange + Ed25519 signatures
- **No central server** — works over LAN, Wi-Fi Direct, WebSocket relay, or DNS tunnel
- **Store-and-forward mesh** — every node carries encrypted messages for the network
- **Offline-first** — messages queue locally and deliver when connectivity returns
- **Proof-of-Work** — anti-spam, no accounts needed
- **PIN protection** — Argon2id hashing, 5 failed attempts = data wipe
- **Panic mode** — instant wipe with decoy data (fake contacts, fake messages about groceries)
- **Encrypted storage** — inbox encrypted at rest with XSalsa20 (VaultKey)
- **Channels** — one-to-many broadcast (like Telegram channels)
- **Group chats** — with reply-to and per-member delivery
- **File transfer** — chunked, encrypted, up to 10 MB
- **FOTA** — over-the-air updates from GitHub Releases
- **Stealth mode** — traffic obfuscation, DNS tunneling, ICMP masking

## Architecture

```
cmd/
  iskra/           — Desktop binary (Go, localhost HTTP + WebView UI)
  iskra-mobile/    — gomobile bindings for Android
  relay/           — WebSocket relay server (stateless forwarder)
  cargo/           — Headless mesh node (silent "clipper" for stealth delivery)

internal/
  crypto/          — XSalsa20-Poly1305, X25519, Ed25519, PoW, obfuscation
  identity/        — Key generation, base58, BIP39 mnemonic (Russian wordlist)
  message/         — Binary protocol: serialize, encrypt, sign, verify
  mesh/            — Transport layer: LAN TCP, WebSocket relay, DNS tunnel, Wi-Fi Direct
  store/           — Bloom filter, hold (store-and-forward), inbox, contacts, groups
  web/             — HTTP API + static UI (HTML/CSS/JS served from embedded filesystem)

android/           — Kotlin WebView wrapper with Wi-Fi Direct mesh support
```

### Transport Layers

| Layer | How | When |
|-------|-----|------|
| **LAN** | Multicast discovery (239.42.42.42:4242) + TCP sync | Devices on same network |
| **Relay** | WebSocket (wss://) | Internet available, NAT traversal |
| **DNS Tunnel** | Base32-encoded messages in DNS queries | HTTP/WS blocked, DNS still works |
| **Wi-Fi Direct** | Android P2P, mDNS service discovery | No network at all — phone-to-phone |

### Message Lifecycle

```
New → [PoW solved] → [Encrypted] → [Signed] → Hold
  → Broadcast to LAN peers
  → Send via relay (or DNS tunnel fallback)
  → Store-and-forward on every sync
  → HopTTL decrements per forward
  → ForwardLimit (15) prevents flooding
  → 3h morgue after exhaustion
  → 30-day kill switch
```

## Quick Start

### Desktop (Windows/Linux)

Download the latest binary from [Releases](https://github.com/elyssov/iskra/releases):

```bash
# Run with default settings (connects to public relay)
./iskra

# Custom port and data directory
./iskra -port 8080 -data ~/.iskra-data

# Restore from mnemonic
./iskra -restore "word1 word2 word3 ... word24"
```

Open `http://localhost:<port>` in your browser. On first launch, you'll get a 24-word mnemonic — **write it down on paper**.

### Android

Download the APK from [Releases](https://github.com/elyssov/iskra/releases) and install. The Go mesh core runs as a background service with a WebView UI.

### Build from Source

**Prerequisites:** Go 1.24+, Android SDK/NDK (for mobile)

```bash
# Desktop binary
go build -o iskra ./cmd/iskra/

# Relay server
go build -o relay ./cmd/relay/

# Android .aar (requires gomobile)
gomobile bind -target=android/arm64,android/arm -androidapi 24 \
  -o android/app/libs/iskra.aar ./cmd/iskra-mobile/

# Android APK (requires Gradle 8.4, JDK 17+)
cd android && gradle assembleRelease
```

### Run Tests

```bash
go test ./internal/...
```

## Security Model

| Layer | Algorithm | Purpose |
|-------|-----------|---------|
| Encryption | XSalsa20-Poly1305 | Message confidentiality + integrity |
| Key exchange | X25519 (Curve25519) | Ephemeral shared secret per message |
| Signatures | Ed25519 | Message authenticity, non-repudiation |
| Key derivation | Argon2id | PIN → encryption key (time=1, mem=64MB) |
| Anti-spam | SHA-256 PoW | 16-bit difficulty, prevents flooding |
| Deduplication | Bloom filter | 1M capacity, 0.1% false positive rate |
| Storage | XSalsa20 (VaultKey) | Inbox encrypted at rest |

**Threat model:** Iskra assumes the network is hostile. All messages are encrypted before leaving the device. The relay server is a blind forwarder — it never sees plaintext. Even if captured, a device's hold contains only encrypted blobs addressed to other people.

**Panic mode:** Long-press the app title (3 seconds) → enter code → all real data is destroyed and replaced with decoy contacts and fake chat history.

## Relay Server

The relay is a minimal WebSocket forwarder. It does not store messages on disk, does not decrypt anything, and assigns random revolutionary aliases to connected peers (Че, Спартак, Робеспьер...).

```bash
# Self-host a relay
go build -o relay ./cmd/relay/
./relay -port 8443

# Or deploy to Render/Fly.io (see render.yaml / fly.toml)
```

## Protocol

Binary message format (230 bytes + payload):

```
[version:1][id:32][recipientID:20][ttl:4][timestamp:8][contentType:1]
[ephemeralPub:32][nonce:24][payloadLen:4][payload:variable]
[authorPub:32][signature:64][powNonce:8]
```

Content types: `0x01` Text, `0x02` DeliveryConfirm, `0x03` GroupText, `0x04` GroupInvite, `0x05` FileChunk, `0x10` ChannelPost.

## Contributing

This is an active project. Issues and pull requests are welcome.

## License

This project is licensed under the [MIT License](LICENSE).

---

*Built with Go, stubbornness, and the belief that communication is a human right.*
