package main

import (
	"fmt"
	"github.com/tomponline/fsclient/fsclient"
)

func main() {
	fmt.Println("Starting...")
	fs := fsclient.NewClient()

	err := fs.Connect()
	if err != nil {
		fmt.Println(err)
		return
	}

	fs.AddFilter("Event-Name HEARTBEAT")
	fs.AddFilter("variable_fsclient true")

	fs.SubcribeEvent("HEARTBEAT")
	fs.SubcribeEvent("Event-Name CHANNEL_PARK")
	fs.SubcribeEvent("Event-Name CHANNEL_CREATE")
	fs.SubcribeEvent("Event-Name CHANNEL_ANSWER")
	fs.SubcribeEvent("Event-Name CHANNEL_HANGUP_COMPLETE")
	fs.SubcribeEvent("Event-Name CHANNEL_PROGRESS")
	fs.SubcribeEvent("Event-Name CHANNEL_EXECUTE")

	hostname, err := fs.API("hostname")
	fmt.Println(hostname)

	for {
		event, err := fs.ReadEvent()

		if err != nil {
			fmt.Println("Got and error: ", err)
			return
		}

		fmt.Print("Action: '", event["Event-Name"], "'\n")

		fmt.Println("Getting hostname...")
		hostname, err := fs.API("hostname")
		fmt.Println("API response: ", hostname)

		if event["Event-Name"] == "CHANNEL_PARK" {
			fmt.Println("Got channel park")
			fs.Execute("answer", "", event["Unique-ID"], true)
			fs.Execute("delay_echo", "", event["Unique-ID"], true)
		}
	}
}
