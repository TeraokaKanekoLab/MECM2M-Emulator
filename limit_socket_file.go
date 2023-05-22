package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

func main() {
	basePath := "/tmp/socketfile"

	var i int
	for i = 0; i < 1000000; i++ {
		socketPath := basePath + strconv.Itoa(i)

		_, err := net.Listen("unix", socketPath)
		if err != nil {
			fmt.Printf("Error creating socket file #%d: %s\n", i, err)
			os.Exit(1)
		}
	}
}
