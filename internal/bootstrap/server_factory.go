package bootstrap

import (
	"net"
	"sync"

	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/internal/rubyserver"
	"gitlab.com/gitlab-org/gitaly/internal/server"
	"google.golang.org/grpc"
)

type serverFactory struct {
	ruby *rubyserver.Server

	rw      sync.Mutex
	servers map[bool]*grpc.Server
}

func NewServerFactory() (*serverFactory, error) {
	ruby, err := rubyserver.Start()
	if err != nil {
		log.Error("start ruby server")

		return nil, err
	}

	return &serverFactory{ruby: ruby}, nil

}

func (s *serverFactory) Stop() {
	s.rw.Lock()
	defer s.rw.Unlock()

	for _, srv := range s.servers {
		if srv == nil {
			continue
		}

		srv.Stop()
	}

	s.ruby.Stop()
}

func (s *serverFactory) GracefulStop() {
	s.rw.Lock()
	defer s.rw.Unlock()

	wg := sync.WaitGroup{}

	for _, srv := range s.servers {
		if srv == nil {
			continue
		}

		wg.Add(1)

		go func(s *grpc.Server) {
			s.GracefulStop()
			wg.Done()
		}(srv)
	}

	wg.Wait()
}

func (s *serverFactory) Serve(l net.Listener) error {
	srv := s.get(isSecure(l.Addr()))

	return srv.Serve(l)
}

func isSecure(addr net.Addr) bool {
	return addr.Network() == "tls"
}

func (s *serverFactory) get(secure bool) *grpc.Server {
	s.rw.Lock()
	defer s.rw.Unlock()

	srv, ok := s.servers[secure]
	if !ok {
		if secure {
			srv = server.NewSecure(s.ruby)
		} else {
			srv = server.NewInsecure(s.ruby)
		}

		s.servers[secure] = srv
	}

	return srv
}
