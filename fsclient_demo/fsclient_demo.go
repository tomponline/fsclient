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
		"HEARTBEAT",
		"CHANNEL_PARK",
		"CHANNEL_CREATE",
		"CHANNEL_ANSWER",
		"CHANNEL_HANGUP_COMPLETE",
		"CHANNEL_PROGRESS",
		"CHANNEL_EXECUTE",
	}

	fs = fsclient.NewClient("127.0.0.1:8021", "ClueCon", filters, subs, 1, initFunc)
	go apiHostnameLoop()
	fsEventHandler()
}

func initFunc(fsclient *fsclient.Client) {
	//Send any commands you want to run on first connect here.
	fmt.Println("fsclient is now initialised")
}

func apiHostname() {
	fmt.Println("Getting hostname...")
	hostname, err := fs.API("hostname")
	if err != nil {
		fmt.Println("API error: ", err)
		return
	}
	fmt.Println("API response: ", hostname)
}

func apiHostnameLoop() {
	for {
		apiHostname()
		time.Sleep(500 * time.Millisecond)
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
