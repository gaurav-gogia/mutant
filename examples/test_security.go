package main

import (
	"fmt"
	"mutant/security"
	"os"
)

// Simple test program to demonstrate anti-debugging features
func main() {
	fmt.Println("=== Mutant Security Feature Test ===\n")

	// Test 1: Basic Debugger Detection
	fmt.Println("1. Testing basic debugger detection...")
	if security.IsDebuggerPresent() {
		fmt.Println("   ⚠️  DEBUGGER DETECTED!")
		fmt.Println("   The program is being debugged.")
	} else {
		fmt.Println("   ✅ No debugger detected (basic check)")
	}

	// Test 2: Advanced Debugger Detection
	fmt.Println("\n2. Testing advanced debugger detection...")
	if security.DetectDebuggerAdvanced() {
		fmt.Println("   ⚠️  DEBUGGER DETECTED (Advanced)!")
		fmt.Println("   Multiple detection methods triggered.")
	} else {
		fmt.Println("   ✅ No debugger detected (advanced check)")
	}

	// Test 3: Secure Random Generation
	fmt.Println("\n3. Testing secure random generation...")
	randomBytes, err := security.SecureRandBytes(16)
	if err != nil {
		fmt.Printf("   ❌ Error: %v\n", err)
	} else {
		fmt.Printf("   ✅ Generated secure random: %x\n", randomBytes)
	}

	// Test 4: Memory Zeroing
	fmt.Println("\n4. Testing secure memory zeroing...")
	sensitiveData := []byte("SuperSecretPassword123")
	fmt.Printf("   Before: %s\n", string(sensitiveData))
	security.SecureZero(sensitiveData)
	fmt.Printf("   After:  %x (all zeros)\n", sensitiveData)

	// Test 5: Constant-Time Comparison
	fmt.Println("\n5. Testing constant-time comparison...")
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

	// Exit with error if debugger detected
	if security.IsDebuggerPresent() {
		fmt.Println("\n⚠️  Exiting due to debugger presence")
		os.Exit(1)
	}
}
