package mesh

import (
	"testing"
	"time"

	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/message"
)

func TestTransport_ConnectAndSendMessage(t *testing.T) {
	// Create two nodes
	seed1 := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	seed2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17,
		16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	kp1 := identity.KeypairFromSeed(seed1)
	kp2 := identity.KeypairFromSeed(seed2)

	peers1 := NewPeerList()
	peers2 := NewPeerList()

	// Node B: listener
	t2 := NewTransport(kp2.Ed25519Pub, 0, peers2)
	received := make(chan *message.Message, 1)
	t2.SetOnMessage(func(msg *message.Message) {
		received <- msg
	})
	if err := t2.Start(); err != nil {
		t.Fatalf("transport2 start failed: %v", err)
	}
	defer t2.Stop()

	// Node A: connect to B
	t1 := NewTransport(kp1.Ed25519Pub, 0, peers1)
	if err := t1.Start(); err != nil {
		t.Fatalf("transport1 start failed: %v", err)
	}
	defer t1.Stop()

	// Connect A → B
	err := t1.ConnectAndSync("127.0.0.1", t2.Port(), nil, nil)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Give handshake time to complete
	time.Sleep(100 * time.Millisecond)

	// Create and send a message from A to B
	rk := message.RecipientKeys{Ed25519Pub: kp2.Ed25519Pub, X25519Pub: kp2.X25519Pub}
	msg, err := message.New(kp1, rk, "Hello from node A!")
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	if err := t1.SendMessage(kp2.Ed25519Pub, msg); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	// Wait for message
	select {
	case got := <-received:
		if got.ID != msg.ID {
			t.Fatal("received message ID mismatch")
		}
		if !got.VerifySignature() {
			t.Fatal("received message signature invalid")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestTransport_Handshake(t *testing.T) {
	seed1 := [32]byte{10}
	seed2 := [32]byte{20}
	kp1 := identity.KeypairFromSeed(seed1)
	kp2 := identity.KeypairFromSeed(seed2)

	peers1 := NewPeerList()
	peers2 := NewPeerList()

	t2 := NewTransport(kp2.Ed25519Pub, 0, peers2)
	t2.SetOnMessage(func(msg *message.Message) {})
	t2.Start()
	defer t2.Stop()

	t1 := NewTransport(kp1.Ed25519Pub, 0, peers1)
	t1.Start()
	defer t1.Stop()

	err := t1.ConnectAndSync("127.0.0.1", t2.Port(), nil, nil)
	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Both should know each other
	if peers1.Count() < 1 {
		t.Fatal("node1 should have peer")
	}
	if peers2.Count() < 1 {
		t.Fatal("node2 should have peer")
	}
}
