// Command desktop-bridge-protocol-gen regenerates the TypeScript and Rust
// mirrors of the desktop bridge wire schema.
package main

import (
	"flag"
	"fmt"
	"log"

	"reasonix/internal/desktopbridge/protocolgen"
)

func main() {
	check := flag.Bool("check", false, "compare generated artifacts without writing them")
	root := flag.String("root", ".", "repository root containing generated artifacts")
	flag.Parse()
	log.SetFlags(0)

	artifacts, err := protocolgen.Generate(*root)
	if err != nil {
		log.Fatal(err)
	}
	if *check {
		if err := protocolgen.Check(*root, artifacts); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Desktop bridge protocol artifacts are up to date.")
		return
	}
	if err := protocolgen.Write(*root, artifacts); err != nil {
		log.Fatal(err)
	}
	for _, artifact := range artifacts {
		fmt.Println(artifact.Path)
	}
}
