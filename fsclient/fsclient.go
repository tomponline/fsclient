package fsclient

import (
	"errors"
	"net"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
	"io"
)

type Client struct {
	eventConn *textproto.Conn
}

func (client *Client) Connect() (err error) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:8021", time.Duration(5*time.Second))
	if err != nil {
		return
	}

	client.eventConn = textproto.NewConn(conn)

	resp, err := client.eventConn.ReadMIMEHeader()
	if err != nil {
		return
	}

	client.eventConn.PrintfLine("auth %s\r\n", "ClueCon")

	if resp, err = client.eventConn.ReadMIMEHeader(); err != nil {
		return
	}

	if resp.Get("Content-Type") == "command/reply" && resp.Get("Reply-Text") == "+OK accepted" {
		return
	}

	return errors.New("Could not authenticate")
}

func (client *Client) AddFilter(arg string) (err error) {
	client.eventConn.PrintfLine("filter %s\r\n", arg)

	resp, err := client.eventConn.ReadMIMEHeader()
	if err != nil {
		return
	}

	if resp.Get("Content-Type") == "command/reply" && resp.Get("Reply-Text") == "+OK" {
		return
	}

	return errors.New("Could not add filter")
}

func (client *Client) SubcribeEvent(arg string) (err error) {
	client.eventConn.PrintfLine("event plain %s\r\n", arg)

	resp, err := client.eventConn.ReadMIMEHeader()
	if err != nil {
		return
	}

	if resp.Get("Content-Type") == "command/reply" && resp.Get("Reply-Text") == "+OK" {
		return
	}

	return errors.New("Could not subcribe to event")
}

func (client *Client) Api(cmd string) (string, error) {
	client.eventConn.PrintfLine("api %s\r\n", cmd)

	resp, err := client.eventConn.ReadMIMEHeader()
	if err != nil {
		return "",err
	}

	if resp.Get("Content-Type") == "api/response" && resp.Get("Content-Length") != "" {
		length, err := strconv.Atoi(resp.Get("Content-Length"))
		if err != nil {
			return "",err
		}

		buf := make([]byte, length)
		if _, err = io.ReadFull(client.eventConn.R, buf); err != nil {
			return "",err
		}
		return string(buf), nil
	}

	return "",errors.New("Could not run command")
}

func (client *Client) Execute(app string, arg string, uuid string, lock bool) (err error) {
	client.eventConn.PrintfLine("sendmsg %s", uuid)
	client.eventConn.PrintfLine("call-command: execute")
	client.eventConn.PrintfLine("execute-app-name: %s", app)

	if arg != "" {
		client.eventConn.PrintfLine("execute-app-arg: %s", arg)
	}

	if lock {
		client.eventConn.PrintfLine("event-lock: true")
	}

	client.eventConn.PrintfLine("")

	resp, err := client.eventConn.ReadMIMEHeader()
	if err != nil {
		return
	}

	if resp.Get("Content-Type") == "command/reply" && resp.Get("Reply-Text") == "+OK" {
		return
	}

	return errors.New("Execute failure")
}

func (client *Client) ReadEvent() (map[string]string, error) {
	resp, err := client.eventConn.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}

	if resp.Get("Content-Type") == "text/event-plain" && resp.Get("Content-Length") != "" {
		_, err := strconv.Atoi(resp.Get("Content-Length"))
		if err != nil {
			return nil, err
		}

		event := make(map[string]string)

		for {
			line, err := client.eventConn.ReadLine()
			if err != nil {
				return event, err
			}

			if line == "" {
				return event, nil
			}

			parts := strings.Split(line, ": ")
			key := parts[0]
			value, err := url.QueryUnescape(parts[1])

			if err != nil {
				return event, err
			}

			event[key] = value
		}

		return event, nil
	}

	return nil, errors.New("Unexpected read error")
}

