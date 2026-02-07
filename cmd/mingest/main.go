package main

import (
	"os"

	"media-ingest/ingest"
)

func main() {
	os.Exit(ingest.Main(os.Args))
}

