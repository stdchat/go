package main

import (
	"fmt"
	"os"

	"stdchat.org/provider"
	"stdchat.org/service"
	"stdchat.org/service/dummy"
)

func main() {
	err := provider.Run(dummy.Protocol,
		func(t service.Transporter) service.Servicer {
			return dummy.NewService(t)
		})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
