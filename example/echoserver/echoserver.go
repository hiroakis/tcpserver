package main

import (
	"context"
	"log"

	"github.com/hiroakis/tcpserver"
)

type EchoServer struct {
}

func (e *EchoServer) OnConnect(c *tcpserver.Connection) {
}

func (e *EchoServer) OnClose(c *tcpserver.Connection, err error) {
}

func (e *EchoServer) OnMessage(c *tcpserver.Connection, msg []byte) {
	c.Send(msg)
}

func main() {
	s, err := tcpserver.NewServer("localhost", "8000", &EchoServer{}, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err := s.Listen(context.Background()); err != nil {
		log.Fatal(err)
	}
}
