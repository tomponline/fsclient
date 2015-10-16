package main

import (
	"fmt"
	"github.com/tomponline/fsclient/fsclient"
)

func main() {
	fmt.Println("Starting...")
	fs := fsclient.Client{}

	err := fs.Connect()
	if err != nil {
		fmt.Println(err)
		return
	}

	fs.AddFilter("Event-Name HEARTBEAT")
	fs.SubcribeEvent("HEARTBEAT")
	fs.SubcribeEvent("Event-Name CHANNEL_CREATE")
	fs.SubcribeEvent("Event-Name CHANNEL_ANSWER")
	fs.SubcribeEvent("Event-Name CHANNEL_HANGUP_COMPLETE")
	fs.SubcribeEvent("Event-Name CHANNEL_PROGRESS")
	fs.SubcribeEvent("Event-Name CHANNEL_EXECUTE")

	hostname, err := fs.Api("hostname")
	fmt.Println(hostname)

	for {
		event, err := fs.ReadEvent()

		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Print("Action: '", event["Event-Name"], "' ID: '", event["Unique-ID"], "'\n")
	}
}
