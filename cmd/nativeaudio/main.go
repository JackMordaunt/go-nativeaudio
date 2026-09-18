package main

import (
	"flag"
	"fmt"
	"os"

	"git.sr.ht/~jackmordaunt/nativeaudio"
	"git.sr.ht/~jackmordaunt/nativeaudio/play"
)

var (
	in  string
	out string
)

func main() {
	flag.StringVar(&in, "in", "", "input audio file")
	flag.StringVar(&out, "out", "", "output pcm file, will play audio if out not specified")
	flag.Parse()
	if err := run(); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if out != "" {
		un, _, err := nativeaudio.Load(in)
		if err != nil {
			return fmt.Errorf("loading audio file: %w", err)
		}
		if err := os.WriteFile(out, un, 0o644); err != nil {
			return fmt.Errorf("writing output pcm file: %w", err)
		}
		return nil
	}
	if err := play.File(in); err != nil {
		return fmt.Errorf("playing audio file: %w", err)
	}
	return nil
}
