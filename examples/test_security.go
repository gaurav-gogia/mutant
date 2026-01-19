package main

import (
	"fmt"
	"mutant/security"
)

// Simple test program to demonstrate security features
func main() {
	fmt.Printf("=== Mutant Security Feature Test ===\n\n")

	// Test 1: Secure Random Generation
	fmt.Println("1. Testing secure random generation...")
	randomBytes, err := security.SecureRandBytes(16)
	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else {
		fmt.Printf("   ✅ Generated secure random: %x\n", randomBytes)
	}

	// Test 2: Memory Zeroing
	fmt.Println("\n2. Testing secure memory zeroing...")
	sensitiveData := []byte("SuperSecretPassword123")
	fmt.Printf("   Before: %s\n", string(sensitiveData))
	security.SecureZero(sensitiveData)
	fmt.Printf("   After:  %x (all zeros)\n", sensitiveData)

	// Test 3: Constant-Time Comparison
	fmt.Println("\n3. Testing constant-time comparison...")
	password1 := []byte("correct_password")
	password2 := []byte("correct_password")
	password3 := []byte("wrong_password")

	if security.SecureCompare(password1, password2) {
		fmt.Println("   ✅ Passwords match (constant-time)")
	}

	if !security.SecureCompare(password1, password3) {
		fmt.Println("   ✅ Passwords don't match (constant-time)")
	}

	fmt.Println("\n=== Test Complete ===")
}
