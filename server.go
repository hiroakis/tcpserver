package tcpserver

import (
	"context"
	"log"
	"net"
	"os"
	"sync"

	"github.com/pkg/errors"
)

// Handler handles TCP connection
type Handler interface {
	OnConnect(*Connection)
	OnMessage(*Connection, []byte)
	OnClose(*Connection, error)
}

// Logger is a Logger interface
type Logger interface {
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

// Server is a TCP server
type Server struct {
	Handler    Handler
	log        Logger
	address    *net.TCPAddr
	wg         *sync.WaitGroup
	shutdownCh chan struct{}
}

var defaultLogger = log.New(os.Stdout, "[tcpserver] ", log.Lshortfile|log.LstdFlags)

// NewServer returns a Server pointer
func NewServer(host, port string, handler Handler, logger Logger) (*Server, error) {
	address, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, errors.Wrap(err, "net.ResolveTCPAddr failed")
	}
	if logger == nil {
		logger = defaultLogger
	}
	return &Server{
		Handler:    handler,
		log:        logger,
		address:    address,
		wg:         new(sync.WaitGroup),
		shutdownCh: make(chan struct{}, 1),
	}, nil
}

func (s *Server) handleNetError(err error) error {
	switch errr := err.(type) {
	case net.Error:
		if errr.Temporary() {
			return nil
		} else {
			return errr
		}
	default:
		return errr
	}
}

const readBufferSize = 4096

func (s *Server) handleConnection(ctx context.Context, c *Connection) error {
	s.log.Printf("Start s.handleConnection %v", c.Conn.RemoteAddr())

	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		c.Conn.Close()
		cancel()
	}()

	errCh := make(chan error, 1)
	s.wg.Add(1)
	go func() {
		defer func() {
			s.wg.Done()
			close(errCh)
		}()

		s.Handler.OnConnect(c)
		s.log.Printf("Connected %v", c.Conn.RemoteAddr())

		for {
			msg := make([]byte, readBufferSize)
			_, err := c.Conn.Read(msg)
			if err != nil {
				if errr := s.handleNetError(err); errr != nil {
					errCh <- errr
					return
				}
			}

			s.log.Printf("Received message %v", c.Conn.RemoteAddr())
			s.Handler.OnMessage(c, msg)
		}
	}()

	select {
	case <-ctx.Done():
		s.log.Printf("Got <-ctx.Done() %v", c.Conn.RemoteAddr())
		s.Handler.OnClose(c, ctx.Err())
		return ctx.Err()
	case err := <-errCh:
		s.log.Printf("Got <-errCh %v", c.Conn.RemoteAddr())
		s.Handler.OnClose(c, err)
		return err
	}
}

func (s *Server) handleListener(ctx context.Context, l *net.TCPListener) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	for {
		conn, err := l.AcceptTCP()
		if err != nil {
			if errr := s.handleNetError(err); errr != nil {
				return errors.Wrap(err, "l.Accept failed")
			}
		}

		s.wg.Add(1)
		go func(_ctx context.Context, _conn *net.TCPConn, _errCh chan error) {
			defer s.wg.Done()

			_errCh <- s.handleConnection(_ctx, &Connection{
				Conn: _conn,
			})
		}(ctx, conn, errCh)

		s.wg.Add(1)
		go func(_ctx context.Context, _conn *net.TCPConn, _errCh chan error) {
			defer s.wg.Done()

			select {
			case <-_ctx.Done():
				s.log.Printf("Got <-ctx.Done from s.handleConnection")
				<-_errCh
				return
			case err := <-_errCh:
				s.log.Printf("Got <-errCh from s.handleConnection %v\n", err)
				return
			}
		}(ctx, conn, errCh)
	}
}

func (s *Server) IsReady() bool {
	select {
	case <-s.readyCh:
		return true
	default:
		return false
	}
}

// Listen acts like Listen for TCP IPv4 networks
func (s *Server) Listen(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
	}()

	s.log.Println("Start net.ListenTCP")
	l, err := net.ListenTCP("tcp", s.address)
	if err != nil {
		return errors.Wrap(err, "net.ListenTCP failed")
	}
	defer l.Close()

	errCh := make(chan error, 1)
	s.wg.Add(1)
	go func() {
		defer func() {
			s.wg.Done()
			close(errCh)
		}()

		errCh <- s.handleListener(ctx, l)
	}()

	select {
	case <-s.shutdownCh:
		close(s.shutdownCh)
		s.log.Println("Got <-s.shutdownCh waiting for all wg is done")
		return nil
	case <-ctx.Done():
		s.log.Println("Got <-ctx.Done from s.handleListener")
		return nil
	case err := <-errCh:
		s.log.Printf("Got <-errCh from s.handleListener %v\n", err)
		return err
	}
}

// Shutdown shutdowns a TCP server
func (s *Server) Shutdown() {
	s.shutdownCh <- struct{}{}
	s.wg.Wait()
	s.log.Println("Shutdown")
}

// Connection is a TCP connection
type Connection struct {
	Conn *net.TCPConn
}

// Send sends a message to a client
func (c *Connection) Send(msg []byte) (int, error) {
	return c.Conn.Write(msg)
}

// Close closes a connection
func (c *Connection) Close() error {
	return c.Conn.Close()
}
