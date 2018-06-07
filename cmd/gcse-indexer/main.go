package main

import (
	"log"

	"github.com/daviddengcn/gcse/configs"
)

func main() {
	log.Println("indexer started...")

	if err := configs.IndexSegments().ClearUndones(); err != nil {
		log.Printf("ClearUndones failed: %v", err)
	}

	if err := clearOutdatedIndex(); err != nil {
		log.Printf("clearOutdatedIndex failed: %v", err)
	}
	doIndex()

	log.Println("indexer exits...")
}
