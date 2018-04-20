//Demo for using sendevent to generate notify messages for Yealink
//phones with XML browser messages for pushing content/actions to handsets.
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
		"Event-Name NOTIFY",
	}

	subs := []string{
		"HEARTBEAT",
		"NOTIFY",
	}

	fs = fsclient.NewClient("127.0.0.1:8021", "ClueCon", filters, subs, initFunc)
	go eventGenerator()
	fsEventHandler()
}

func initFunc(fsclient *fsclient.Client) {
	//Send any commands you want to run on first connect here.
	fmt.Println("fsclient is now initialised")
}

func eventGenerator() {
	for {
		params := make(map[string]string)
		params["profile"] = "external"

		//Use these to trigger a provision request with optional reboot.
		//params["content-type"] = "application/simple-message-summary"
		//params["event-string"] = "check-sync"
		//params["event-string"] = "check-sync;reboot=false"

		//Use these to send XML browser events to handsets
		params["content-type"] = "application/xml"
		params["event-string"] = "Yealink-xml"
		params["user"] = "test.user@domain.local"
		params["host"] = "domain.local"
		params["from-uri"] = "sip:test.user@domain.local"

		//Extract the dial string for a registered handset.
		params["to-uri"] = "sip:test.user@10.1.30.170:5060;transport=TCP;fs_nat=yes;fs_path=sip%3Atest.user%4010.1.30.170%3A12300%3Btransport%3DTCP"

		//Example for triggering remote DND on
		msg := `<YealinkIPPhoneConfiguration Beep="yes" setType="config">
			<Item>features.dnd.enable = 1</Item>
		</YealinkIPPhoneConfiguration>`

		//Example for pushing message to hand set screen.
		/*msg := `<YealinkIPPhoneStatus Beep="yes">
		   <Session>0</Session>
		   <Message
		   		Index="1"
		   		Type="alert"
		   		Timeout="1"
		   		Icon="Message"
		   		Size="normal"
		   		Align="center"
		   		Color="blue">Hello World</Message>
		</YealinkIPPhoneStatus>`*/

		//Example for sending multi-line screen to handset screen.
		/*msg := `<YealinkIPPhoneFormattedTextScreen
		doneAction="http://10.1.0.105/menu.php"
		Beep="yes"
		Timeout="60"
		LockIn="no">

		<Scroll>
		<Line Size="normal">Current Conditions</Line>
		<Line Size="small">Temperature: 2.1°C</Line>
		<Line Size="small">Pressure: 100.7 kPa falling</Line>
		<Line Size="small">Wind: S 31 km/h gust 45 km/h</Line>
		<Line Size="normal">Monday, 13 January Forecast</Line>
		<Line Size="small">Cloudy. A few showers beginning near noon.</Line>
		<Line Size="small">Wind southwest 30 km/h. High plus 4.</Line>
		</Scroll>

		</YealinkIPPhoneFormattedTextScreen>`*/

		res, err := fs.SendEvent("NOTIFY", params, msg)
		fmt.Println("Send Event Result: ", res, err)
		time.Sleep(2000 * time.Millisecond)
	}
}

func fsEventHandler() {
	for {
		event := fs.NextEvent()
		fmt.Print("Action: '", event["Event-Name"], "'\n")
		if event["Event-Name"] == "NOTIFY" {
			//Example of seeing the NOTIFY messages you generate with SendEvent().
			fmt.Println("Got notify:", event)
		}
	}
}
