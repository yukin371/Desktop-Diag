//go:build windows && phase4diag

// This development tool compares the product ICMP wrapper with controlled payloads.
package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/yukin371/desktop-diag/internal/winapi"
)

// main prints original errors without modifying network or security configuration.
func main() {
	if len(os.Args) != 2 || net.ParseIP(os.Args[1]).To4() == nil {
		fmt.Fprintln(os.Stderr, "usage: icmp-go.exe gateway")
		os.Exit(2)
	}
	// target/payload keep the comparison limited to two hosts and two fixed payloads.
	for _, target := range []string{"127.0.0.1", os.Args[1]} {
		for _, payload := range []string{"Desktop-Diag ICMP probe payload!", "abcdefghijklmnopqrstuvwabcdefghi"} {
			if err := send(target, payload); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}
}

// send opens and closes one ICMP handle so each case has independent state.
func send(target, payload string) error {
	// handle/err preserve original handle-creation failures.
	handle, err := winapi.IcmpCreateFile()
	if err != nil {
		return fmt.Errorf("open ICMP handle: %w", err)
	}
	// result/sendErr are printed together so a zero-valued reply cannot imply success.
	result, sendErr := winapi.IcmpSendEcho(handle, net.ParseIP(target), []byte(payload), time.Second)
	fmt.Printf("target=%s payload=%q size=%d result=%+v error=%v\n", target, payload, len(payload), result, sendErr)
	if err := winapi.IcmpCloseHandle(handle); err != nil {
		return fmt.Errorf("close ICMP handle: %w", err)
	}
	return nil
}
