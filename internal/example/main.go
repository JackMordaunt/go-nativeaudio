package main

import (
	"fmt"
	"os"

	"git.sr.ht/~jackmordaunt/nativeaudio"
)

func main() {
	by, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(fmt.Errorf("reading file: %w", err))
	}
	d, err := nativeaudio.New()
	if err != nil {
		panic(fmt.Errorf("creating decoder: %w", err))
	}
	defer d.Close()
	data, format, err := d.Decode(by)
	if err != nil {
		panic(fmt.Errorf("decoding audio: %w", err))
	}
	fmt.Printf("%+v\n", format)
	if err := os.WriteFile("raw.pcm", data, 0644); err != nil {
		panic(fmt.Errorf("dumping pcm: %w", err))
	}
}
