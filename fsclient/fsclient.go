//Package fsclient provides a client for the Freeswitch Event Socket.
package fsclient

import (
	"errors"
	"io"
	"log"
	"net"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var errDisconnected = errors.New("Disconnected")
var logPrefix = "fsclient: "

//Client represents a Freeswitch client. Contains the event socket connection.
type Client struct {
	eventConn *textproto.Conn
	addr      string
	password  string
	cmdResCh  chan cmdRes
	EventCh   chan map[string]string
	filters   []string
	subs      []string
	connMu    *sync.Mutex
	initFunc  func(*Client)
}

//cmdRes is a response structure for Freeswitch commands.
type cmdRes struct {
	body string
	err  error
}

//NewClient creates a new Freeswitch client with filters, subscriptions and an init function.
func NewClient(addr string, password string, filters []string, subs []string, eventBufSize int, initFunc func(*Client)) *Client {
	fs := &Client{
		addr:     addr,
		password: password,
		EventCh:  make(chan map[string]string, eventBufSize),
		filters:  filters,
		subs:     subs,
		connMu:   &sync.Mutex{},
		initFunc: initFunc,
	}

	go fs.readHandler()
	return fs
}

//Connect establishes a connection with the local Freeswitch server.
func (client *Client) connect() (err error) {

	//If the cmd response channel has been previously initialised, this is an
	//indication that we are reconnecting, so we need to close the channel so
	//that any threads waiting for a response to an API request are returned
	//and they will then release the lock on the connection so we can modify it.
	if client.cmdResCh != nil {
		close(client.cmdResCh)
	}

	//Now get a lock on the connection as we need to mutate connection state.
	client.connMu.Lock()
	defer client.connMu.Unlock()

	//Make sure old connection is closed and reset to initial state.
	//This will stop further API commands from being sent.
	if client.eventConn != nil {
		client.eventConn.Close()
		client.eventConn = nil
		client.cmdResCh = nil
	}

	//Connect to Freeswitch Event Socket.
	conn, err := net.DialTimeout("tcp", client.addr, time.Duration(5*time.Second))
	if err != nil {
		return
	}

	//Convert the raw TCP connection to a textproto connection.
	eventConn := textproto.NewConn(conn)

	//Read the welcome message.
	resp, err := eventConn.ReadMIMEHeader()
	if err != nil {
		return
	}

	//Send authentication request to server.
	eventConn.PrintfLine("auth %s\r\n", client.password)
	if resp, err = eventConn.ReadMIMEHeader(); err != nil {
		return
	}

	//Check the command was processed OK.
	if resp.Get("Reply-Text") == "+OK accepted" {
		//The connection is now ready to be used, make available for use
		//and also create a new cmd response channel.
		client.eventConn = eventConn
		client.cmdResCh = make(chan cmdRes)
		return
	}

	return errors.New("Authentication failed: " + resp.Get("Reply-Text"))
}

//setupFilters configures which events to receive from Freeswitch.
func (client *Client) setupFilters() {
	log.Print(logPrefix, "Setting up filters...")
	for _, filter := range client.filters {
		if err := client.addFilter(filter); err != nil {
			log.Print(logPrefix, err)
		}
	}

	for _, sub := range client.subs {
		if err := client.subcribeEvent(sub); err != nil {
			log.Print(logPrefix, err)
		}
	}
	log.Print(logPrefix, "Filters setup")
}

//AddFilter specifies event types to listen for.
//Note, this is not a filter out but rather a "filter in," that is, when a
//filter is applied only the filtered values are received.
//Multiple filters on a socket connection are allowed.
func (client *Client) addFilter(arg string) (err error) {
	client.connMu.Lock()
	defer client.connMu.Unlock()

	//Send filter command to server.
	client.eventConn.PrintfLine("filter %s\r\n", arg)
	body, err := client.readCmdRes()

	//Check the command was processed OK.
	if strings.HasPrefix(body, "+OK") {
		return
	}

	return errors.New("Failed filter add '" + arg + "': " + body)
}

//SubcribeEvent enables events by class or all.
func (client *Client) subcribeEvent(arg string) (err error) {
	client.connMu.Lock()
	defer client.connMu.Unlock()

	//Send event command to server.
	client.eventConn.PrintfLine("event plain %s\r\n", arg)
	body, _ := client.readCmdRes()

	//Check the command was processed OK.
	if strings.HasPrefix(body, "+OK") {
		return
	}

	return errors.New("Failed subcribe to event '" + arg + "': " + body)
}

//readCmdRes waits until Freeswitch delivers a command response message.
//It will block until a message arrives or until the cmdResCh channel is closed
//indicating that we have been disconnected from the server, at which point a
//errDisconnected response is delivered instead.
func (client *Client) readCmdRes() (string, error) {
	res := <-client.cmdResCh
	if res.body == "" && res.err == nil {
		return "", errDisconnected
	}
	return res.body, res.err
}

//API sends an api command (blocking mode).
func (client *Client) API(cmd string) (string, error) {
	client.connMu.Lock()
	defer client.connMu.Unlock()

	//If the command response channel is not intialised then it means we
	//are not connected. So no point in sending a command.
	if client.cmdResCh == nil {
		return "", errDisconnected
	}
	client.eventConn.PrintfLine("api %s\r\n", cmd)
	return client.readCmdRes()
}

//Execute is used to execute dialplan applications on a channel.
func (client *Client) Execute(app string, arg string, uuid string, lock bool) (string, error) {
	client.connMu.Lock()
	defer client.connMu.Unlock()

	//If the command response channel is not intialised then it means we
	//are not connected. So no point in sending a command.
	if client.cmdResCh == nil {
		return "", errDisconnected
	}

	//Send execute command to server.
	client.eventConn.PrintfLine("sendmsg %s", uuid)
	client.eventConn.PrintfLine("call-command: execute")
	client.eventConn.PrintfLine("execute-app-name: %s", app)

	if arg != "" {
		client.eventConn.PrintfLine("execute-app-arg: %s", arg)
	}

	if lock {
		client.eventConn.PrintfLine("event-lock: true")
	}

	client.eventConn.PrintfLine("") //Empty line indicates end of command.
	return client.readCmdRes()
}

//SendEvent is used to send an event into the event system.
func (client *Client) SendEvent(eventName string, eventParams map[string]string, eventBody string) (string, error) {
	client.connMu.Lock()
	defer client.connMu.Unlock()

	//If the command response channel is not intialised then it means we
	//are not connected. So no point in sending a command.
	if client.cmdResCh == nil {
		return "", errDisconnected
	}

	//Send sendevent command to server.
	client.eventConn.PrintfLine("sendevent %s", eventName)
	for paramKey, paramVal := range eventParams {
		client.eventConn.PrintfLine("%s: %s", paramKey, paramVal)
	}

	if eventBody != "" {
		client.eventConn.PrintfLine("Content-Length: %d", len(eventBody))
		client.eventConn.PrintfLine("") //Empty line indicates end of header.
		client.eventConn.W.WriteString(eventBody)
		client.eventConn.W.Flush()

	} else {
		client.eventConn.PrintfLine("") //Empty line indicates end of command.
	}

	return client.readCmdRes()
}

//readHandler receives messages from Freeswitch and distributes them.
func (client *Client) readHandler() {
ConnectLoop:
	for {
		log.Print(logPrefix, "Connecting...")
		err := client.connect()
		if err != nil {
			log.Print(logPrefix, "Failed to connect: ", err)
			time.Sleep(2 * time.Second)
			continue ConnectLoop
		}
		log.Print(logPrefix, "Connected OK")
		go client.setupFilters()
		go client.initFunc(client)

		//Read next message off Freeswitch connection.
	MsgLoop:
		for {
			resp, err := client.eventConn.ReadMIMEHeader()
			if err != nil {
				log.Print(logPrefix, "Read failure: ", err)
				continue ConnectLoop
			}

			if resp.Get("Content-Type") == "text/event-plain" {
				if err := client.handleEventMsg(resp); err != nil {
					continue ConnectLoop
				}
			} else if resp.Get("Content-Type") == "api/response" {
				if err := client.handleAPIMsg(resp); err != nil {
					continue ConnectLoop
				}
			} else if resp.Get("Content-Type") == "command/reply" {
				client.cmdResCh <- cmdRes{
					body: resp.Get("Reply-Text"),
					err:  err,
				}
				continue MsgLoop
			} else if resp.Get("Content-Type") == "text/disconnect-notice" {
				log.Print(logPrefix, "Freeswitch shutting down...")
				continue MsgLoop //Get any final messages before it disconnects.
			} else {
				log.Print(logPrefix, resp.Get("Content-Type"))
			}
		}
	}
}

//deliverEvent sends an event to the EventCh channel, logs discarded messages.
func (client *Client) deliverEvent(event map[string]string) {
	chanLen := len(client.EventCh)
	select {
	case client.EventCh <- event:
	case <-time.After(1 * time.Second): //Wait up to 1s to deliver to channel.
		log.Print(logPrefix, "Error Event channel blocked (", chanLen,
			" items), discarded Event: ", event["Unique-ID"], " ", event["Event-Name"])
	}
}

//handleEventMsg processes event messages received from Freeswitch.
func (client *Client) handleEventMsg(resp textproto.MIMEHeader) error {
	event := make(map[string]string)
	bodyLength := 0

	//Check that Content-Length is numeric.
	_, err := strconv.Atoi(resp.Get("Content-Length"))
	if err != nil {
		log.Print(logPrefix, "Invalid Content-Length", err)
		return err
	}

	for {
		//Read each line of the event and store into map.
		line, err := client.eventConn.ReadLine()
		if err != nil {
			log.Print(logPrefix, "Event Read failure: ", err)
			return err
		}

		if line == "" { //Empty line means end of event headers.
			//If the bodyLength has been set greater than zero, then read the
			//body of the event into a special key called "body".
			if bodyLength > 0 {
				buf := make([]byte, bodyLength)
				client.eventConn.Reader.R.Read(buf)
				event["body"] = string(buf)
			}
			client.deliverEvent(event)
			return err
		}

		parts := strings.Split(line, ": ") //Split "Key: value"
		key := parts[0]
		value, err := url.QueryUnescape(parts[1])

		if err != nil {
			log.Print(logPrefix, "Parse failure: ", err)
			return err
		}

		//If the header key indicates there is additional content at the end
		//of this message, then convert it to an integer for reading later.
		if key == "Content-Length" || key == "content-length" {
			bodyLength, _ = strconv.Atoi(value)
		}

		event[key] = value
	}
}

//handleAPIMsg processes API response messages received from Freeswitch.
//It delivers the response to the waiting function via the cmdResCh channel.
func (client *Client) handleAPIMsg(resp textproto.MIMEHeader) error {
	//Check that Content-Length is numeric.
	length, err := strconv.Atoi(resp.Get("Content-Length"))
	if err != nil {
		log.Print(logPrefix, "Invalid Content-Length", err)
		client.cmdResCh <- cmdRes{body: "", err: err}
		return err
	}

	//Read Content-Length bytes into a buffer and convert to string.
	buf := make([]byte, length)
	if _, err = io.ReadFull(client.eventConn.R, buf); err != nil {
		log.Print(logPrefix, "API Read failure: ", err)
	}
	client.cmdResCh <- cmdRes{body: string(buf), err: err}
	return err
}
