package encryption

import (
	"testing"
)

func TestGeneratePasswordHash(t *testing.T) {
	password := "password123"
	hash, err := GeneratePasswordHash(password)
	if err != nil {
		t.Fatalf("GeneratePasswordHash failed: %v", err)
	}

	if hash == "" {
		t.Errorf("Expected hash to not be empty")
	}
}

func TestComparePassword(t *testing.T) {
	password := "password123"
	hash, err := GeneratePasswordHash(password)
	if err != nil {
		t.Fatalf("GeneratePasswordHash failed: %v", err)
	}

	match, err := ComparePassword(password, hash)
	if err != nil {
		t.Fatalf("ComparePassword failed: %v", err)
	}

	if !match {
		t.Errorf("Expected password to match hash")
	}

	wrongPassword := "wrongpassword"
	match, err = ComparePassword(wrongPassword, hash)
	if err != nil {
		t.Fatalf("ComparePassword failed for wrong password: %v", err)
	}
	if match {
		t.Errorf("Expected wrong password to not match hash")
	}

	invalidHash := "invalidhash"
	_, err = ComparePassword(password, invalidHash)
	if err == nil {
		t.Errorf("Expected error for invalid hash format")
	}
}

func TestComparePassword_InvalidHashFormat(t *testing.T) {
	password := "password123"
	invalidHash := "$argon2id$v=19$m=65536,t=1,p=4$invalidSalt" // Missing hash part
	match, err := ComparePassword(password, invalidHash)
	if err == nil {
		t.Errorf("Expected error for invalid hash format, got nil")
	}
	if match {
		t.Errorf("Expected match to be false for invalid hash format")
	}
}
