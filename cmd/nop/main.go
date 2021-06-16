package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("Phony Halo service")
	for {
		time.Sleep(1 * time.Hour)
	}
}
