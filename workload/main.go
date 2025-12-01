package main

import (
	"crypto/sha256"
	"time"
)

func main() {
	for {
		data := []byte(time.Now().String())
		_ = sha256.Sum256(data)
		time.Sleep(10 * time.Millisecond)
	}
}
