//This demo tests the bgapi functionality in fsclient.
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
		"Event-Name BACKGROUND_JOB",
	}

	subs := []string{
		"HEARTBEAT",
		"BACKGROUND_JOB",
	}

	fs = fsclient.NewClient("127.0.0.1:8021", "ClueCon", filters, subs, 1, initFunc)
	go bgapiHostnameLoop()
	fsEventHandler()
}

func initFunc(fsclient *fsclient.Client) {
	//Send any commands you want to run on first connect here.
	fmt.Println("fsclient is now initialised")
}

func bgapiHostname() {
	fmt.Println("Getting hostname...")
	jobUUID, err := fs.BackgroundAPI("hostname")
	if err != nil {
		fmt.Println("API error: ", err)
		return
	}
	fmt.Print("BGAPI response: ", jobUUID, "\n")
}

func bgapiHostnameLoop() {
	for {
		bgapiHostname()
		time.Sleep(500 * time.Millisecond)
	}
}

func fsEventHandler() {
	for {
		event := <-fs.EventCh
		fmt.Print("Action: '", event["Event-Name"], "'\n")

		if event["Event-Name"] == "BACKGROUND_JOB" {
			fmt.Print("Got background job result: ", event["body-string"], "\n")
		}
	}
}
