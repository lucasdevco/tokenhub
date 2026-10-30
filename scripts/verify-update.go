// Verify the signed update archive before publishing its manifest.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 5 {
		log.Fatal("usage: verify-update MANIFEST CONFIG VERSION ARCH")
	}
	var manifest struct {
		Version, URL, Signature string
		Size                    int64
	}
	var config struct {
		Updates struct{ PublicKey string }
	}
	readJSON := func(path string, value any) {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		if err := json.Unmarshal(data, value); err != nil {
			log.Fatal(err)
		}
	}
	readJSON(os.Args[1], &manifest)
	readJSON(os.Args[2], &config)
	name := "tokenhub-" + os.Args[3] + "-darwin-" + os.Args[4] + ".tar.gz"
	expected := "https://github.com/lucasdevco/tokenhub/releases/download/v" + os.Args[3] + "/" + name
	if manifest.Version != os.Args[3] || manifest.URL != expected {
		log.Fatal("update manifest version or download URL does not match this release")
	}
	archive, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[1]), name))
	if err != nil {
		log.Fatal(err)
	}
	key, err := base64.StdEncoding.DecodeString(config.Updates.PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		log.Fatal("invalid update public key")
	}
	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	digest := sha256.Sum256(archive)
	if err != nil || int64(len(archive)) != manifest.Size || !ed25519.Verify(key, digest[:], signature) {
		log.Fatal("update archive size or signature verification failed")
	}
	fmt.Println("Verified signed update:", name)
}
