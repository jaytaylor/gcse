package main

import (
	"log"

	"github.com/daviddengcn/gcse/configs"
)

func main() {
	log.Println("indexer started...")

	if err := configs.IndexSegments().ClearUndones(); err != nil {
		log.Printf("Indexer: ClearUndones failed: %v", err)
	}

	if err := clearOutdatedIndex(); err != nil {
		log.Printf("Indexer: clearOutdatedIndex failed: %v", err)
	}
	if !doIndex() {
		log.Fatal("Indexer encountered one or more problems")
	}

	log.Println("Indexer finished OK, exiting.")
}
