package main

import (
	"fmt"
	"github.com/tomponline/fsclient/fsclient"
	"time"
)

var fs *fsclient.Client

func main() {
	fmt.Println("Starting...")

	filters := []string{
		"Event-Name HEARTBEAT",
		"variable_fsclient true",
	}

	subs := []string{
		"Event-Name HEARTBEAT",
		"Event-Name CHANNEL_PARK",
		"Event-Name CHANNEL_CREATE",
		"Event-Name CHANNEL_ANSWER",
		"Event-Name CHANNEL_HANGUP_COMPLETE",
		"Event-Name CHANNEL_PROGRESS",
		"Event-Name CHANNEL_EXECUTE",
	}

	fs = fsclient.NewClient("127.0.0.1:8021", "ClueCon", filters, subs, initFunc)
	go apiHostnameLoop()
	fsEventHandler()
}

func initFunc() {
	fmt.Println("fsclient is now initialised")
}

func apiHostname() {
	fmt.Println("Getting hostname...")
	hostname, _ := fs.API("hostname")
	fmt.Println("API response: ", hostname)
}

func apiHostnameLoop() {
	for {
		fmt.Println("Getting hostname...")
		hostname, err := fs.API("hostname")
		fmt.Println("API response: ", hostname, err)
		time.Sleep(1 * time.Second)
	}
}

func fsEventHandler() {
	for {
		event := <-fs.EventCh
		fmt.Print("Action: '", event["Event-Name"], "'\n")

		go apiHostname()

		if event["Event-Name"] == "CHANNEL_PARK" {
			fmt.Println("Got channel park")
			fs.Execute("answer", "", event["Unique-ID"], true)
			fs.Execute("delay_echo", "", event["Unique-ID"], true)
		}
	}
}
