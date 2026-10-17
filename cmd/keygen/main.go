package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func main() {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	key := "rj_" + hex.EncodeToString(b)
	fmt.Printf("Guarde a chave; não a versione.\nAPI key: %s\nAPI_KEY_SHA256=%x\n", key, sha256.Sum256([]byte(key)))
}
