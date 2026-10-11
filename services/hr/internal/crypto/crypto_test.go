package crypto

import (
	"bytes"
	"testing"
)

func TestCryptoRoundTrip(t *testing.T) {
	encKey := bytes.Repeat([]byte{0x42}, 32)
	blindKey := "test-secret-blind-key-12345"

	svc, err := New(encKey, blindKey)
	if err != nil {
		t.Fatalf("failed to create crypto service: %v", err)
	}

	plaintext := "3201012345678901"
	aad := "school-1:nik"

	ciphertext, bidx, err := svc.EncryptWithIndex(plaintext, aad)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}
	if len(ciphertext) == 0 {
		t.Fatal("empty ciphertext")
	}
	if bidx == "" {
		t.Fatal("empty blind index")
	}

	// Decrypt
	decrypted, err := svc.Decrypt(ciphertext, aad)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}

	// Blind index must be deterministic
	bidx2 := svc.BlindIndex(plaintext, aad)
	if bidx2 != bidx {
		t.Fatalf("blind index not deterministic: %q vs %q", bidx, bidx2)
	}

	// Different plaintext produces different blind index
	bidxDiff := svc.BlindIndex("9999999999999999", aad)
	if bidxDiff == bidx {
		t.Fatal("different plaintext produced identical blind index")
	}

	// Wrong AAD fails decryption
	if _, err := svc.Decrypt(ciphertext, "wrong-aad"); err == nil {
		t.Fatal("decryption should have failed with wrong AAD")
	}

	// Wrong key fails decryption
	wrongKeySvc, _ := New(bytes.Repeat([]byte{0x99}, 32), blindKey)
	if _, err := wrongKeySvc.Decrypt(ciphertext, aad); err == nil {
		t.Fatal("decryption should have failed with wrong key")
	}
}

func TestCryptoInvalidKeys(t *testing.T) {
	if _, err := New([]byte("short"), "long-enough-blind-key"); err == nil {
		t.Fatal("expected error with short enc key")
	}
	if _, err := New(bytes.Repeat([]byte{0x01}, 32), "short"); err == nil {
		t.Fatal("expected error with short blind key")
	}
}
