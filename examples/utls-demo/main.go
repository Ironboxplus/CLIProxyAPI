package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func main() {
	// Example 1: Using SetProxy with uTLS fingerprinting
	fmt.Println("=== Example 1: SetProxy with uTLS ===")
	cfg := &config.SDKConfig{
		TLSFingerprint: "chrome_latest",
	}

	client := &http.Client{}
	client = util.SetProxy(cfg, client)

	// Make a request
	resp, err := client.Get("https://tls.peet.ws/api/all")
	if err != nil {
		log.Printf("Request failed: %v\n", err)
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Response status: %s\n", resp.Status)
		fmt.Printf("Response body (first 200 chars): %s\n\n", body[:min(200, len(body))])
	}

	// Example 2: Direct uTLS application
	fmt.Println("=== Example 2: Direct uTLS Application ===")
	client2 := &http.Client{}
	client2 = util.ApplyUTLSToClient(client2, util.FingerprintFirefoxLatest)

	resp2, err := client2.Get("https://www.howsmyssl.com/a/check")
	if err != nil {
		log.Printf("Request failed: %v\n", err)
	} else {
		defer resp2.Body.Close()
		body, _ := io.ReadAll(resp2.Body)
		fmt.Printf("Response status: %s\n", resp2.Status)
		fmt.Printf("Response body (first 200 chars): %s\n\n", body[:min(200, len(body))])
	}

	// Example 3: Testing different fingerprints
	fmt.Println("=== Example 3: Testing Different Fingerprints ===")
	fingerprints := []util.TLSFingerprint{
		util.FingerprintChrome120,
		util.FingerprintFirefox120,
		util.FingerprintSafari16,
		util.FingerprintEdge85,
		util.FingerprintiOSLatest,
	}

	for _, fp := range fingerprints {
		client := &http.Client{}
		client = util.ApplyUTLSToClient(client, fp)

		resp, err := client.Get("https://www.google.com")
		if err != nil {
			fmt.Printf("Fingerprint %s: FAILED - %v\n", fp, err)
		} else {
			resp.Body.Close()
			fmt.Printf("Fingerprint %s: SUCCESS - %s\n", fp, resp.Status)
		}
	}

	fmt.Println("\n=== All examples completed ===")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
