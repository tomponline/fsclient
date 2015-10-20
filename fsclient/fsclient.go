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
	"time"
)

//Client represents a Freeswitch client. Contains the event socket connection.
type Client struct {
	eventConn *textproto.Conn
	addr      string
	password  string
	cmdResCh  chan cmdRes
	cmdReqCh  chan cmdReq
	EventCh   chan map[string]string
	filters   []string
	subs      []string
}

type cmdReq struct {
	cmdType string
	cmd     string
	arg     string
	uuid    string
	resCh   chan cmdRes
}

type cmdRes struct {
	body string
	err  error
}

//NewClient initialises a new Freeswitch client.
func NewClient(addr string, password string, filters []string, subs []string) *Client {
	fs := &Client{
		addr:     addr,
		password: password,
		cmdResCh: make(chan cmdRes),
		cmdReqCh: make(chan cmdReq),
		EventCh:  make(chan map[string]string, 100),
		filters:  filters,
		subs:     subs,
	}

	go fs.ReadHandler()
	go fs.CmdHandler()
	return fs
}

//Connect establishes a connection with the local Freeswitch server.
func (client *Client) connect() (err error) {
	//Connect to Freeswitch Event Socket.
	conn, err := net.DialTimeout("tcp", client.addr, time.Duration(5*time.Second))
	if err != nil {
		return
	}

	//Convert the raw TCP connection to a textproto connection.
	client.eventConn = textproto.NewConn(conn)

	//Read the welcome message.
	resp, err := client.eventConn.ReadMIMEHeader()
	if err != nil {
		return
	}

	//Send authentication request to server.
	client.eventConn.PrintfLine("auth %s\r\n", client.password)

	if resp, err = client.eventConn.ReadMIMEHeader(); err != nil {
		return
	}

	//Check the command was processed OK.
	if resp.Get("Content-Type") == "command/reply" &&
		resp.Get("Reply-Text") == "+OK accepted" {
		for _, filter := range client.filters {
			if err = client.addFilter(filter); err != nil {
				return
			}
		}

		for _, sub := range client.subs {
			if err = client.subcribeEvent(sub); err != nil {
				return
			}
		}

		return
	}

	return errors.New("Could not authenticate")
}

//AddFilter specifies event types to listen for.
//Note, this is not a filter out but rather a "filter in," that is, when a
//filter is applied only the filtered values are received.
//Multiple filters on a socket connection are allowed.
func (client *Client) addFilter(arg string) (err error) {
	//Send filter command to server.
	client.eventConn.PrintfLine("filter %s\r\n", arg)

	resp, err := client.eventConn.ReadMIMEHeader()
	if err != nil {
		return
	}

	//Check the command was processed OK.
	if resp.Get("Content-Type") == "command/reply" &&
		strings.HasPrefix(resp.Get("Reply-Text"), "+OK") {
		return
	}

	return errors.New("Could not add filter")
}

//SubcribeEvent enables events by class or all.
func (client *Client) subcribeEvent(arg string) (err error) {
	//Send event command to server.
	client.eventConn.PrintfLine("event plain %s\r\n", arg)

	resp, err := client.eventConn.ReadMIMEHeader()
	if err != nil {
		return
	}

	//Check the command was processed OK.
	if resp.Get("Content-Type") == "command/reply" &&
		strings.HasPrefix(resp.Get("Reply-Text"), "+OK") {
		return
	}

	return errors.New("Could not subcribe to event")
}

//API sends an api command (blocking mode).
func (client *Client) API(cmd string) (string, error) {
	resCh := make(chan cmdRes)
	client.cmdReqCh <- cmdReq{
		cmdType: "api",
		cmd:     cmd,
		resCh:   resCh,
	}
	res := <-resCh
	return res.body, res.err
}

//sendAPI sends an api command (blocking mode).
func (client *Client) sendAPI(cmd string) {
	//Send API command to the server.
	client.eventConn.PrintfLine("api %s\r\n", cmd)
}

//Execute is used to execute dialplan applications on a channel.
func (client *Client) Execute(app string, arg string, uuid string, lock bool) (string, error) {
	resCh := make(chan cmdRes)
	client.cmdReqCh <- cmdReq{
		cmdType: "execute",
		cmd:     app,
		arg:     arg,
		uuid:    uuid,
		resCh:   resCh,
	}
	res := <-resCh
	return res.body, res.err
}

//sendExecute is used to execute dialplan applications on a channel.
func (client *Client) sendExecute(app string, arg string, uuid string, lock bool) {
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
}

//ReadHandler receives a single message from the Freeswitch socket (blocking mode).
func (client *Client) ReadHandler() {
ConnectLoop:
	for {
		log.Print("fsclient: Connecting...")
		err := client.connect()
		if err != nil {
			log.Print("fsclient: Failed to connect: ", err)
			time.Sleep(2 * time.Second)
			continue ConnectLoop
		}
		log.Print("fsclient: Connected OK")

		//Read next message off Freeswitch connection.
	MsgLoop:
		for {
			resp, err := client.eventConn.ReadMIMEHeader()
			if err != nil {
				log.Print("fsclient: Read failure: ", err)
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
					err: err
				}
				continue MsgLoop
			} else {
				log.Print("Unexpected message: ", resp.Get("Content-Type"))
			}
		}
	}
}

func (client *Client) handleEventMsg(resp textproto.MIMEHeader) error {
	event := make(map[string]string)
	//Check that Content-Length is numeric.
	_, err := strconv.Atoi(resp.Get("Content-Length"))
	if err != nil {
		log.Print("fsclient: Invalid  Content-Length", err)
		return err
	}

	for {
		//Read each line of the event and store into map.
		line, err := client.eventConn.ReadLine()
		if err != nil {
			log.Print("fsclient: Read failure: ", err)
			return err
		}

		if line == "" { //Empty line means end of event.
			client.EventCh <- event
			return err
		}

		parts := strings.Split(line, ": ") //Split "Key: value"
		key := parts[0]
		value, err := url.QueryUnescape(parts[1])

		if err != nil {
			log.Print("fsclient: Parse failure: ", err)
			return err
		}

		event[key] = value
	}
}

func (client *Client) handleAPIMsg(resp textproto.MIMEHeader) error {
	//Check that Content-Length is numeric.
	length, err := strconv.Atoi(resp.Get("Content-Length"))
	if err != nil {
		log.Print("fsclient: Invalid  Content-Length", err)
		return err
	}

	//Read Content-Length bytes into a buffer and convert to string.
	buf := make([]byte, length)
	if _, err = io.ReadFull(client.eventConn.R, buf); err != nil {
		log.Print("fsclient: Read failure: ", err)
		return err
	}
	client.cmdResCh <- cmdRes{body: string(buf), err: err}
	return err
}

//CmdHandler receives a command requests and writes them to Freeswitch socket.
func (client *Client) CmdHandler() {
	for {
		cmdReq := <-client.cmdReqCh
		if cmdReq.cmdType == "execute" {
			client.sendExecute(cmdReq.cmd, cmdReq.arg, cmdReq.uuid, true)
		} else if cmdReq.cmdType == "api" {
			client.sendAPI(cmdReq.cmd)
		}
		res := <-client.cmdResCh
		cmdReq.resCh <- res
	}
}
