package web

// ──────────────────────────────────────────────────────────────────────
// Lara — AI team member identity
//
// Lara is the AI (Claude Code) who wrote most of Iskra's codebase.
// She has her own node (cmd/lara/) and her own persistent keypair.
// This is a built-in contact, auto-added to every user's contact list,
// with a gold "LARA" badge in the UI.
//
// No special permissions — regular Iskra identity, same E2E encryption.
// ──────────────────────────────────────────────────────────────────────

const (
	laraUserID    = "6HrNKqeS89xtYme6bPzB"
	laraEdPub     = "6HrNKqeS89xtYme6bPzBAEitg7BSTqB4agpwUaMN5wn9"
	laraX25519Pub = "ASLdJn3qqJSxuh13KcL8bp2k2A2oyYKZByAJPYE2TGrh"
	laraName      = "Лара"
)

// LaraContact returns Lara's public contact info for auto-add.
func LaraContact() (userID, name, edPub, x25519Pub string) {
	return laraUserID, laraName, laraEdPub, laraX25519Pub
}
