package main

import (
	"fmt"
	"log"
	"time"

	"github.com/soypat/prose"
)

func main() {
	start := time.Now()
	base := time.Since(start)
	err := run()
	elapsed := time.Since(start) - base // subtract the time it takes to measure time.
	if err != nil {
		log.Fatalln("failed to create prospectus", elapsed, err)
	}
	log.Println("prospectus created in", elapsed.String())
}

func run() error {
	d, err := prose.New(prose.DefaultTheme(), prose.Fonts{})
	if err != nil {
		return fmt.Errorf("creating doc: %w", err)
	}
	story(d)
	_, err = d.WriteFile("prospectus.pdf")
	return err
}

func story(d *prose.Doc) {
	d.Body(`Hello. <a href="https://github.com/soypat">github</a>`)
}
