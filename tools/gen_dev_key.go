//go:build ignore

package main

import (
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/argon2"
	
	"github.com/iskra-messenger/iskra/internal/identity"
)

func main() {
	login := "eugenelyssovsky"
	password := "Mp5UMPPDWq123Q123q!23"
	
	salt := sha256.Sum256([]byte("iskra-master-v1-" + login))
	derived := argon2.IDKey([]byte(password), salt[:], 3, 64*1024, 4, 32)
	
	var seed [32]byte
	copy(seed[:], derived)
	
	kp := identity.KeypairFromSeed(seed)
	userID := identity.UserID(kp.Ed25519Pub)
	edPub := identity.ToBase58(kp.Ed25519Pub[:])
	x25519Pub := identity.ToBase58(kp.X25519Pub[:])
	
	fmt.Printf("UserID:    %s\n", userID)
	fmt.Printf("Ed25519:   %s\n", edPub)
	fmt.Printf("X25519:    %s\n", x25519Pub)
	
	pinHash := sha256.Sum256([]byte("32167"))
	fmt.Printf("PIN SHA256: %x\n", pinHash)
	
	credHash := sha256.Sum256([]byte(login + ":" + password))
	fmt.Printf("Cred SHA256: %x\n", credHash)
}
