package server

import (
	"bytes"
	"io"
	"strconv"
	"strings"

	"log"
	"net"
	"net/url"
	"sync"
)

// HandlerFunc handler
type HandlerFunc func(req *Request)

// Server class
type Server struct {
	addr string

	mu sync.RWMutex

	handlers map[string]HandlerFunc
}

// Request class
type Request struct {
	Conn        net.Conn
	QueryParams url.Values
	PathParams  map[string]string
	Headers     map[string]string
	Body        []byte
}

// NewServer can create new servers
func NewServer(addr string) *Server {
	return &Server{addr: addr, handlers: make(map[string]HandlerFunc)}

}

// Register path
func (s *Server) Register(path string, handler HandlerFunc) {
	s.mu.RLock()
	s.handlers[path] = handler
	s.mu.RUnlock()
}

// Start is main function
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Print(err)
		return err
	}

	defer func() {
		if cerr := listener.Close(); cerr != nil {

			if err == nil {
				err = cerr
				return
			}
			log.Print(cerr)
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Print(err)
			continue
		}

		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, (1024 * 8))
	for {
		bufferSize, err := conn.Read(buf)
		if err == io.EOF {
			log.Printf("%s", buf[:bufferSize])
		}

		if err != nil {
			log.Println(err)
			return
		}

		var req Request
		data := buf[:bufferSize]

		endOfLine := []byte{'\r', '\n'}
		endIndex := bytes.Index(data, endOfLine)

		if endIndex == -1 {
			return
		}

		lineEnders := []byte{'\r', '\n', '\r', '\n'}
		lineIndex := bytes.Index(data, lineEnders)
		if endIndex == -1 {
			return
		}

		headersLine := string(data[endIndex:lineIndex])
		headers := strings.Split(headersLine, "\r\n")[1:]

		headerTo := make(map[string]string)
		for _, v := range headers {
			headerLine := strings.Split(v, ": ")
			headerTo[headerLine[0]] = headerLine[1]
		}

		req.Headers = headerTo

		newData := string(data[lineIndex:])
		newData = strings.Trim(newData, "\r\n")

		req.Body = []byte(newData)

		reqLine := string(data[:endIndex])
		parts := strings.Split(reqLine, " ")

		if len(parts) != 3 {
			return
		}

		path, version := parts[1], parts[2]
		if version != "HTTP/1.1" {
			return
		}

		decode, err := url.PathUnescape(path)
		if err != nil {
			log.Println(err)
			return
		}

		uri, err := url.ParseRequestURI(decode)
		if err != nil {
			log.Println(err)
			return
		}

		req.Conn = conn
		req.QueryParams = uri.Query()

		var handler = func(req *Request) { conn.Close() }

		s.mu.RLock()
		pathParameters, hr := s.validate(uri.Path)
		if hr != nil {
			handler = hr
			req.PathParams = pathParameters
		}
		s.mu.RUnlock()

		handler(&req)
	}
}

func (s *Server) validate(path string) (map[string]string, HandlerFunc) {
	strRoutes := make([]string, len(s.handlers))
	i := 0
	for k := range s.handlers {
		strRoutes[i] = k
		i++
	}

	headerTo := make(map[string]string)

	for i := 0; i < len(strRoutes); i++ {
		flag := false
		route := strRoutes[i]
		partsRoute := strings.Split(route, "/")
		pRotes := strings.Split(path, "/")

		for j, v := range partsRoute {
			if v != "" {
				f := v[0:1]
				l := v[len(v)-1:]
				if f == "{" && l == "}" {
					headerTo[v[1:len(v)-1]] = pRotes[j]
					flag = true
				} else if pRotes[j] != v {

					strs := strings.Split(v, "{")
					if len(strs) > 0 {
						key := strs[1][:len(strs[1])-1]
						headerTo[key] = pRotes[j][len(strs[0]):]
						flag = true
					} else {
						flag = false
						break
					}
				}
				flag = true
			}
		}
		if flag {
			if hr, found := s.handlers[route]; found {
				return headerTo, hr
			}
			break
		}
	}

	return nil, nil

}

// Response common answer
func (s *Server) Response(body string) string {
	return "HTTP/1.1 200 OK\r\n" +
		"Content-Length: " + strconv.Itoa(len(body)) + "\r\n" +
		"Content-Type: text/html\r\n" +
		"Connection: close\r\n" +
		"\r\n" + body
} 