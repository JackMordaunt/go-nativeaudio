package main

import (
	"flag"
	"fmt"
	"os"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

var in string

func main() {
	flag.StringVar(&in, "in", "", "input audio file to play")
	flag.Parse()
	if err := run(); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	return nativeaudio.Play(in)
}
