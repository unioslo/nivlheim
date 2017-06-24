package main

import (
	"fmt"
	"time"
)

func main() {
	for {
		fmt.Printf("Hei verden!\n")
		time.Sleep(time.Second * 60)
	}
}
