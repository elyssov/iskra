package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iskra-messenger/iskra/internal/security"
)

func main() {
	// Create temp directory — never touches real ~/.iskra
	demoDir := filepath.Join(os.TempDir(), fmt.Sprintf("iskra-panic-demo-%d", time.Now().UnixNano()))
	os.MkdirAll(demoDir, 0700)
	defer os.RemoveAll(demoDir)

	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║     🔥 ИСКРА — Демонстрация режима ПАНИКА       ║")
	fmt.Println("║                                                  ║")
	fmt.Println("║   Работает во временной директории.              ║")
	fmt.Println("║   Ваши реальные данные НЕ затрагиваются.        ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Printf("\n📁 Демо-директория: %s\n\n", demoDir)

	// === PHASE 1: Create fake "real" data ===
	fmt.Println("━━━ Фаза 1: Создаём «настоящие» данные подпольщика ━━━\n")

	// Seed
	seed := make([]byte, 32)
	rand.Read(seed)
	os.WriteFile(filepath.Join(demoDir, "seed.key"), seed, 0600)
	fmt.Println("  ✓ seed.key (ключ личности)")

	// Contacts — real revolutionary contacts
	realContacts := []map[string]interface{}{
		{"name": "Координатор Москва", "user_id": "abc123", "pubkey": "secret_pub_1"},
		{"name": "Ячейка Питер", "user_id": "def456", "pubkey": "secret_pub_2"},
		{"name": "Правозащитник", "user_id": "ghi789", "pubkey": "secret_pub_3"},
	}
	data, _ := json.MarshalIndent(realContacts, "", "  ")
	os.WriteFile(filepath.Join(demoDir, "contacts.json"), data, 0600)
	fmt.Println("  ✓ contacts.json (3 контакта: Координатор, Ячейка, Правозащитник)")

	// Inbox — compromising messages
	realInbox := map[string][]map[string]interface{}{
		"abc123": {
			{"text": "Завтра в 15:00 на Пушкинской", "outgoing": false, "timestamp": time.Now().Add(-2 * time.Hour).Unix()},
			{"text": "Принял, буду с плакатами", "outgoing": true, "timestamp": time.Now().Add(-1 * time.Hour).Unix()},
			{"text": "Адвокат на связи: +7-XXX-XXX-XX-XX", "outgoing": false, "timestamp": time.Now().Add(-30 * time.Minute).Unix()},
		},
		"def456": {
			{"text": "У нас 200 человек подтвердили участие", "outgoing": false, "timestamp": time.Now().Add(-3 * time.Hour).Unix()},
			{"text": "Маршрут: Невский → Дворцовая", "outgoing": true, "timestamp": time.Now().Add(-2 * time.Hour).Unix()},
		},
		"ghi789": {
			{"text": "Если задержат — молчите, требуйте адвоката", "outgoing": false, "timestamp": time.Now().Add(-5 * time.Hour).Unix()},
			{"text": "Памятка по правам при задержании в чате", "outgoing": false, "timestamp": time.Now().Add(-4 * time.Hour).Unix()},
		},
	}
	data, _ = json.MarshalIndent(realInbox, "", "  ")
	os.WriteFile(filepath.Join(demoDir, "inbox.json"), data, 0600)
	os.MkdirAll(filepath.Join(demoDir, "inbox"), 0700)
	fmt.Println("  ✓ inbox.json (компрометирующая переписка)")

	// Hold
	os.MkdirAll(filepath.Join(demoDir, "hold"), 0700)
	for i := 0; i < 5; i++ {
		msg := make([]byte, 200)
		rand.Read(msg)
		os.WriteFile(filepath.Join(demoDir, "hold", fmt.Sprintf("msg_%d.msg", i)), msg, 0600)
	}
	fmt.Println("  ✓ hold/ (5 сообщений в трюме)")

	// Groups
	os.WriteFile(filepath.Join(demoDir, "groups.json"), []byte(`{"groups":[{"name":"Координация 29 марта","id":"secret"}],"messages":{}}`), 0600)
	fmt.Println("  ✓ groups.json (группа «Координация 29 марта»)")

	// Show what's on disk
	fmt.Println("\n📂 Содержимое ДО паники:")
	listDir(demoDir, "  ")

	// === PHASE 2: Panic! ===
	fmt.Println("\n━━━ Фаза 2: ПАНИКА! Код 159 введён ━━━\n")
	fmt.Println("  ⚠️  Уничтожение данных...")
	fmt.Println()

	start := time.Now()
	security.WipeAll(demoDir)
	wipeTime := time.Since(start)

	fmt.Printf("  🗑  Все файлы перезаписаны случайными данными (3 прохода) и удалены\n")
	fmt.Printf("  ⏱  Время уничтожения: %v\n", wipeTime.Round(time.Millisecond))

	fmt.Println("\n📂 Содержимое ПОСЛЕ уничтожения:")
	listDir(demoDir, "  ")

	// === PHASE 3: Generate decoy ===
	fmt.Println("\n━━━ Фаза 3: Генерация шума (decoy) ━━━\n")

	start = time.Now()
	security.GenerateDecoy(demoDir)
	decoyTime := time.Since(start)

	fmt.Printf("  ⏱  Время генерации: %v\n\n", decoyTime.Round(time.Millisecond))

	fmt.Println("📂 Содержимое ПОСЛЕ генерации шума:")
	listDir(demoDir, "  ")

	// === PHASE 4: Show decoy content ===
	fmt.Println("\n━━━ Что видит товарищ майор ━━━\n")

	// Contacts
	data, _ = os.ReadFile(filepath.Join(demoDir, "contacts.json"))
	var decoyContacts []map[string]interface{}
	json.Unmarshal(data, &decoyContacts)
	fmt.Println("  👥 Контакты:")
	for _, c := range decoyContacts {
		fmt.Printf("     • %s\n", c["name"])
	}

	// Inbox
	data, _ = os.ReadFile(filepath.Join(demoDir, "inbox.json"))
	var decoyInbox map[string][]map[string]interface{}
	json.Unmarshal(data, &decoyInbox)
	fmt.Println("\n  💬 Переписки:")
	for uid, msgs := range decoyInbox {
		// Find contact name
		name := uid[:8] + "..."
		for _, c := range decoyContacts {
			if c["user_id"] == uid {
				name = c["name"].(string)
				break
			}
		}
		fmt.Printf("\n  ── %s (%d сообщений) ──\n", name, len(msgs))
		// Show last 5 messages
		start := len(msgs) - 5
		if start < 0 {
			start = 0
		}
		for _, m := range msgs[start:] {
			arrow := "  ←"
			if out, ok := m["outgoing"].(bool); ok && out {
				arrow = "  →"
			}
			text := m["text"].(string)
			if len(text) > 60 {
				text = text[:60] + "..."
			}
			fmt.Printf("     %s %s\n", arrow, text)
		}
	}

	// Hold
	holdEntries, _ := os.ReadDir(filepath.Join(demoDir, "hold"))
	fmt.Printf("\n  📦 Трюм: %d зашифрованных сообщений (рандомный шум)\n", len(holdEntries))

	fmt.Println("\n╔══════════════════════════════════════════════════╗")
	fmt.Println("║  ✅ Демо завершено                               ║")
	fmt.Println("║                                                  ║")
	fmt.Println("║  Реальные данные: Координатор, Ячейка, маршруты  ║")
	fmt.Println("║  → УНИЧТОЖЕНЫ (3 прохода перезаписи)             ║")
	fmt.Println("║                                                  ║")
	fmt.Println("║  Майор видит: Маша, Дима, Алёна, Серёга          ║")
	fmt.Println("║  → Плов, Дюна, футбол, Барсик на столе           ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
}

func listDir(dir string, indent string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Printf("%s(пусто)\n", indent)
		return
	}
	if len(entries) == 0 {
		fmt.Printf("%s(пусто)\n", indent)
		return
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			subEntries, _ := os.ReadDir(path)
			fmt.Printf("%s📁 %s/ (%d файлов)\n", indent, e.Name(), len(subEntries))
		} else {
			info, _ := e.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			fmt.Printf("%s📄 %s (%d байт)\n", indent, e.Name(), size)
		}
	}
}
